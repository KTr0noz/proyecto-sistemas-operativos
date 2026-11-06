package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type Mensaje struct {
	Tipo              string `json:"tipo"`
	Contenido         string `json:"contenido,omitempty"`
	Usuario           string `json:"usuario,omitempty"`
	Password          string `json:"password,omitempty"`
	Periodo           int    `json:"periodo,omitempty"`
	IntentosRestantes int    `json:"intentos_restantes,omitempty"`
}

type Config struct {
	IPPermitida        string
	Puerto             string
	IntentosFallidos   int
	UsuariosPermitidos []string
}

func main() {
	cfg, err := cargarConfig("servidor.conf")
	if err != nil {
		fmt.Printf("Error cargando servidor.conf: %v\n", err)
		return
	}

	listener, err := net.Listen("tcp", ":"+cfg.Puerto)
	if err != nil {
		fmt.Printf("Error iniciando socket servidor: %v\n", err)
		return
	}
	defer listener.Close()

	fmt.Println("==================================================")
	fmt.Printf("Servidor Operativos escuchando en puerto :%s\n", cfg.Puerto)
	fmt.Printf("IP Permitida: %s | Intentos permitidos: %d\n", cfg.IPPermitida, cfg.IntentosFallidos)
	fmt.Println("Esperando conexiones remotas...")
	fmt.Println("==================================================")

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Printf("Error aceptando conexion: %v\n", err)
			continue
		}
		go atenderCliente(conn, cfg)
	}
}

func atenderCliente(conn net.Conn, cfg *Config) {
	defer conn.Close()

	clienteIP, _, _ := net.SplitHostPort(conn.RemoteAddr().String())
	fmt.Printf("[+] Nueva conexion desde: %s\n", clienteIP)

	if cfg.IPPermitida != "*" && cfg.IPPermitida != clienteIP {
		fmt.Printf("[-] Conexion rechazada: IP %s no autorizada en servidor.conf\n", clienteIP)
		return
	}

	decoder := json.NewDecoder(conn)
	encoder := json.NewEncoder(conn)

	// Proceso de autenticacion con control de intentos fallidos
	var usuarioAutenticado string
	var periodoReporte int
	intentosRestantes := cfg.IntentosFallidos

	for intentosRestantes > 0 {
		var msg Mensaje
		if err := decoder.Decode(&msg); err != nil {
			return
		}

		if msg.Tipo != "auth" {
			encoder.Encode(Mensaje{Tipo: "auth_error", Contenido: "Protocolo invalido: se requiere autenticacion"})
			return
		}

		if validarCredenciales(msg.Usuario, msg.Password) && usuarioPermitido(msg.Usuario, cfg.UsuariosPermitidos) {
			usuarioAutenticado = msg.Usuario
			periodoReporte = msg.Periodo
			if periodoReporte <= 0 {
				periodoReporte = 5
			}
			encoder.Encode(Mensaje{Tipo: "auth_ok", Contenido: "Autenticacion exitosa"})
			fmt.Printf("[+] Usuario '%s' autenticado exitosamente desde %s\n", usuarioAutenticado, clienteIP)
			break
		}

		intentosRestantes--
		fmt.Printf("[-] Intento fallido para '%s'. Quedan %d intentos.\n", msg.Usuario, intentosRestantes)

		if intentosRestantes <= 0 {
			encoder.Encode(Mensaje{
				Tipo:              "auth_error",
				Contenido:         "Limite de intentos fallidos superado. Conexion cerrada.",
				IntentosRestantes: 0,
			})
			return
		}

		encoder.Encode(Mensaje{
			Tipo:              "auth_error",
			Contenido:         "Usuario o contrasena incorrectos.",
			IntentosRestantes: intentosRestantes,
		})
	}

	// Ejecucion en paralelo: Goroutine 1 (Reporte de Recursos)
	sesionActiva := make(chan struct{})
	defer close(sesionActiva)

	go func() {
		numReporte := 0
		ticker := time.NewTicker(time.Duration(periodoReporte) * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				numReporte++
				metricas := obtenerMetricasLinux()
				reporte := fmt.Sprintf("REPORTE #%d: %s", numReporte, metricas)
				if err := encoder.Encode(Mensaje{Tipo: "reporte", Contenido: reporte}); err != nil {
					return
				}
			case <-sesionActiva:
				return
			}
		}
	}()

	// Ejecucion en paralelo: Goroutine 2 / Bucle Principal (Ejecucion remota de comandos)
	for {
		var msg Mensaje
		if err := decoder.Decode(&msg); err != nil {
			break
		}

		if msg.Tipo == "comando" {
			cmd := strings.TrimSpace(msg.Contenido)
			if cmd == "bye" {
				fmt.Printf("[*] Usuario '%s' envio 'bye'. Cerrando sesion.\n", usuarioAutenticado)
				encoder.Encode(Mensaje{Tipo: "bye", Contenido: "Sesion finalizada."})
				break
			}

			fmt.Printf("[*] Ejecutando comando de '%s': %s\n", usuarioAutenticado, cmd)
			salida, err := exec.Command("bash", "-c", cmd).CombinedOutput()
			if err != nil && len(salida) == 0 {
				salida = []byte(fmt.Sprintf("Error ejecutando: %v\n", err))
			}

			encoder.Encode(Mensaje{Tipo: "salida", Contenido: string(salida)})
		}
	}
}

func cargarConfig(ruta string) (*Config, error) {
	file, err := os.Open(ruta)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	cfg := &Config{
		IPPermitida:      "*",
		Puerto:           "2306",
		IntentosFallidos: 3,
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		linea := strings.TrimSpace(scanner.Text())
		if linea == "" || strings.HasPrefix(linea, "#") {
			continue
		}
		partes := strings.SplitN(linea, "=", 2)
		if len(partes) != 2 {
			continue
		}
		clave := strings.TrimSpace(partes[0])
		valor := strings.TrimSpace(partes[1])

		switch clave {
		case "ip_permitida":
			cfg.IPPermitida = valor
		case "puerto":
			cfg.Puerto = valor
		case "intentos_fallidos":
			if n, err := strconv.Atoi(valor); err == nil {
				cfg.IntentosFallidos = n
			}
		case "usuarios_permitidos":
			lista := strings.Split(valor, ",")
			for _, u := range lista {
				cfg.UsuariosPermitidos = append(cfg.UsuariosPermitidos, strings.TrimSpace(u))
			}
		}
	}
	return cfg, nil
}

func validarCredenciales(usuario, password string) bool {
	file, err := os.Open("usuarios.db")
	if err != nil {
		return false
	}
	defer file.Close()

	// Hash SHA-256 de la contrasena
	hashBytes := sha256.Sum256([]byte(password))
	hashHex := hex.EncodeToString(hashBytes[:])

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		linea := strings.TrimSpace(scanner.Text())
		if linea == "" || strings.HasPrefix(linea, "#") {
			continue
		}
		partes := strings.Split(linea, ":")
		if len(partes) >= 2 && partes[0] == usuario {
			return partes[1] == hashHex
		}
	}
	return false
}

func usuarioPermitido(usuario string, permitidos []string) bool {
	if len(permitidos) == 0 {
		return true
	}
	for _, p := range permitidos {
		if p == usuario || p == "*" {
			return true
		}
	}
	return false
}

func obtenerMetricasLinux() string {
	cpu := leerCPU()
	ram := leerRAM()
	procesos := contarProcesos()
	disco := leerDisco()

	return fmt.Sprintf("CPU: %s | RAM: %s | Procesos: %d | Disco: %s", cpu, ram, procesos, disco)
}

func leerCPU() string {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return "N/D"
	}
	lineas := strings.Split(string(data), "\n")
	if len(lineas) == 0 {
		return "N/D"
	}
	campos := strings.Fields(lineas[0])
	if len(campos) < 5 || campos[0] != "cpu" {
		return "N/D"
	}

	var total uint64
	for _, val := range campos[1:] {
		n, _ := strconv.ParseUint(val, 10, 64)
		total += n
	}
	idle, _ := strconv.ParseUint(campos[4], 10, 64)
	if total == 0 {
		return "0%"
	}
	uso := (total - idle) * 100 / total
	return fmt.Sprintf("%d%%", uso)
}

func leerRAM() string {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return "N/D"
	}
	var totalKB, libreKB uint64
	for _, linea := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(linea, "MemTotal:") {
			campos := strings.Fields(linea)
			if len(campos) >= 2 {
				totalKB, _ = strconv.ParseUint(campos[1], 10, 64)
			}
		} else if strings.HasPrefix(linea, "MemAvailable:") {
			campos := strings.Fields(linea)
			if len(campos) >= 2 {
				libreKB, _ = strconv.ParseUint(campos[1], 10, 64)
			}
		}
	}
	if totalKB == 0 {
		return "N/D"
	}
	usadaMB := (totalKB - libreKB) / 1024
	totalMB := totalKB / 1024
	pct := ((totalKB - libreKB) * 100) / totalKB
	return fmt.Sprintf("%d%% (%dMB/%dMB)", pct, usadaMB, totalMB)
}

func contarProcesos() int {
	entradas, err := os.ReadDir("/proc")
	if err != nil {
		return 0
	}
	conteo := 0
	for _, e := range entradas {
		if e.IsDir() {
			if _, err := strconv.Atoi(e.Name()); err == nil {
				conteo++
			}
		}
	}
	return conteo
}

func leerDisco() string {
	out, err := exec.Command("df", "-h", "/").CombinedOutput()
	if err != nil {
		return "N/D"
	}
	lineas := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lineas) >= 2 {
		campos := strings.Fields(lineas[1])
		if len(campos) >= 5 {
			return campos[4] // Columna de porcentaje de uso
		}
	}
	return "N/D"
}
