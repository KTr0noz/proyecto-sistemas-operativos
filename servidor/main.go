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
	"sync"
	"time"
)

type Mensaje struct {
	Tipo              string  `json:"tipo"`
	Contenido         string  `json:"contenido,omitempty"`
	Usuario           string  `json:"usuario,omitempty"`
	Password          string  `json:"password,omitempty"`
	Periodo           int     `json:"periodo,omitempty"`
	IntentosRestantes int     `json:"intentos_restantes,omitempty"`
	PorcentajeCPU     float64 `json:"porcentaje_cpu,omitempty"`
	PorcentajeRAM     float64 `json:"porcentaje_ram,omitempty"`
	PorcentajeDisco   float64 `json:"porcentaje_disco,omitempty"`
}

type Configuracion struct {
	IPPermitida        string
	Puerto             string
	IntentosFallidos   int
	UsuariosPermitidos []string
}

func main() {
	config, err := cargarConfiguracion("servidor.conf")
	if err != nil {
		fmt.Printf("Error cargando servidor.conf: %v\n", err)
		return
	}

	oyente, err := net.Listen("tcp", ":"+config.Puerto)
	if err != nil {
		fmt.Printf("Error iniciando socket servidor: %v\n", err)
		return
	}
	defer oyente.Close()

	fmt.Println("==================================================")
	fmt.Printf("Servidor Operativos escuchando en puerto :%s\n", config.Puerto)
	fmt.Printf("IP Permitida: %s | Intentos permitidos: %d\n", config.IPPermitida, config.IntentosFallidos)
	fmt.Println("Esperando conexiones remotas...")
	fmt.Println("==================================================")

	for {
		conexion, err := oyente.Accept()
		if err != nil {
			fmt.Printf("Error aceptando conexion: %v\n", err)
			continue
		}
		go atenderCliente(conexion, config)
	}
}

func atenderCliente(conexion net.Conn, config *Configuracion) {
	defer conexion.Close()

	ipCliente, _, _ := net.SplitHostPort(conexion.RemoteAddr().String())
	fmt.Printf("[+] Nueva conexion desde: %s\n", ipCliente)

	if config.IPPermitida != "*" && config.IPPermitida != ipCliente {
		fmt.Printf("[-] Conexion rechazada: IP %s no autorizada en servidor.conf\n", ipCliente)
		return
	}

	decodificador := json.NewDecoder(conexion)
	codificador := json.NewEncoder(conexion)
	var bloqueoEnvio sync.Mutex

	enviarMensaje := func(msg Mensaje) error {
		bloqueoEnvio.Lock()
		defer bloqueoEnvio.Unlock()
		return codificador.Encode(msg)
	}

	usuarioAutenticado, periodoReporte, ok := autenticarCliente(decodificador, enviarMensaje, config, ipCliente)
	if !ok {
		return
	}

	sesionActiva := make(chan struct{})
	defer close(sesionActiva)

	// Hilo paralelo encargado de enviar metricas reales cada N segundos
	go func() {
		numeroReporte := 0
		temporizador := time.NewTicker(time.Duration(periodoReporte) * time.Second)
		defer temporizador.Stop()

		for {
			select {
			case <-temporizador.C:
				numeroReporte++
				textoMetricas, pctCPU, pctRAM, pctDisco := obtenerMetricasLinux()
				textoReporte := fmt.Sprintf("Reporte #%d | %s", numeroReporte, textoMetricas)
				err := enviarMensaje(Mensaje{
					Tipo:            "reporte",
					Contenido:       textoReporte,
					PorcentajeCPU:   pctCPU,
					PorcentajeRAM:   pctRAM,
					PorcentajeDisco: pctDisco,
				})
				if err != nil {
					return
				}
			case <-sesionActiva:
				return
			}
		}
	}()

	// Bucle principal de la sesion: recibe y ejecuta comandos UNIX remotos
	for {
		var mensajeRecibido Mensaje
		if err := decodificador.Decode(&mensajeRecibido); err != nil {
			break
		}

		if mensajeRecibido.Tipo != "comando" {
			continue
		}

		comando := strings.TrimSpace(mensajeRecibido.Contenido)
		if strings.EqualFold(comando, "bye") {
			fmt.Printf("[*] Usuario '%s' envio 'bye'. Cerrando sesion.\n", usuarioAutenticado)
			enviarMensaje(Mensaje{Tipo: "bye", Contenido: "Sesion finalizada."})
			break
		}

		fmt.Printf("[*] Ejecutando comando de '%s': %s\n", usuarioAutenticado, comando)
		salida, err := exec.Command("bash", "-c", comando).CombinedOutput()
		if err != nil && len(salida) == 0 {
			salida = []byte(fmt.Sprintf("Error ejecutando: %v\n", err))
		}

		enviarMensaje(Mensaje{Tipo: "salida", Contenido: string(salida)})
	}
}

func autenticarCliente(
	decodificador *json.Decoder,
	enviarMensaje func(Mensaje) error,
	config *Configuracion,
	ipCliente string,
) (string, int, bool) {
	intentosRestantes := config.IntentosFallidos

	for intentosRestantes > 0 {
		var mensajeRecibido Mensaje
		if err := decodificador.Decode(&mensajeRecibido); err != nil {
			return "", 0, false
		}

		if mensajeRecibido.Tipo != "auth" {
			enviarMensaje(Mensaje{Tipo: "auth_error", Contenido: "Protocolo invalido: se requiere autenticacion"})
			return "", 0, false
		}

		if validarCredenciales(mensajeRecibido.Usuario, mensajeRecibido.Password) &&
			usuarioPermitido(mensajeRecibido.Usuario, config.UsuariosPermitidos) {
			periodo := mensajeRecibido.Periodo
			if periodo <= 0 {
				periodo = 5
			}
			enviarMensaje(Mensaje{Tipo: "auth_ok", Contenido: "Autenticacion exitosa"})
			fmt.Printf("[+] Usuario '%s' autenticado exitosamente desde %s\n", mensajeRecibido.Usuario, ipCliente)
			return mensajeRecibido.Usuario, periodo, true
		}

		intentosRestantes--
		fmt.Printf("[-] Intento fallido para '%s'. Quedan %d intentos.\n", mensajeRecibido.Usuario, intentosRestantes)

		if intentosRestantes <= 0 {
			enviarMensaje(Mensaje{
				Tipo:              "auth_error",
				Contenido:         "Limite de intentos fallidos superado. Conexion cerrada.",
				IntentosRestantes: 0,
			})
			return "", 0, false
		}

		enviarMensaje(Mensaje{
			Tipo:              "auth_error",
			Contenido:         "Usuario o contrasena incorrectos.",
			IntentosRestantes: intentosRestantes,
		})
	}

	return "", 0, false
}

func cargarConfiguracion(ruta string) (*Configuracion, error) {
	archivo, err := os.Open(ruta)
	if err != nil {
		return nil, err
	}
	defer archivo.Close()

	config := &Configuracion{
		IPPermitida:      "*",
		Puerto:           "2306",
		IntentosFallidos: 3,
	}

	escaner := bufio.NewScanner(archivo)
	for escaner.Scan() {
		linea := strings.TrimSpace(escaner.Text())
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
			config.IPPermitida = valor
		case "puerto":
			config.Puerto = valor
		case "intentos_fallidos":
			if n, err := strconv.Atoi(valor); err == nil {
				config.IntentosFallidos = n
			}
		case "usuarios_permitidos":
			lista := strings.Split(valor, ",")
			for _, u := range lista {
				config.UsuariosPermitidos = append(config.UsuariosPermitidos, strings.TrimSpace(u))
			}
		}
	}
	return config, nil
}

func validarCredenciales(usuario, password string) bool {
	archivo, err := os.Open("usuarios.db")
	if err != nil {
		return false
	}
	defer archivo.Close()

	hashBytes := sha256.Sum256([]byte(password))
	hashHex := hex.EncodeToString(hashBytes[:])

	escaner := bufio.NewScanner(archivo)
	for escaner.Scan() {
		linea := strings.TrimSpace(escaner.Text())
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
	for _, permitido := range permitidos {
		if permitido == usuario || permitido == "*" {
			return true
		}
	}
	return false
}

func obtenerMetricasLinux() (string, float64, float64, float64) {
	textoCPU, pctCPU := leerCPU()
	textoRAM, pctRAM := leerRAM()
	procesos := contarProcesos()
	textoDisco, pctDisco := leerDisco()

	resumen := fmt.Sprintf("CPU: %s | RAM: %s | Procesos: %d | Disco: %s", textoCPU, textoRAM, procesos, textoDisco)
	return resumen, pctCPU, pctRAM, pctDisco
}

func leerCPU() (string, float64) {
	datos, err := os.ReadFile("/proc/stat")
	if err != nil {
		return "N/D", 0
	}
	lineas := strings.Split(string(datos), "\n")
	if len(lineas) == 0 {
		return "N/D", 0
	}
	campos := strings.Fields(lineas[0])
	if len(campos) < 5 || campos[0] != "cpu" {
		return "N/D", 0
	}

	var total uint64
	for _, valor := range campos[1:] {
		n, _ := strconv.ParseUint(valor, 10, 64)
		total += n
	}
	inactivo, _ := strconv.ParseUint(campos[4], 10, 64)
	if total == 0 {
		return "0%", 0
	}
	porcentaje := float64(total-inactivo) * 100.0 / float64(total)
	return fmt.Sprintf("%.0f%%", porcentaje), porcentaje
}

func leerRAM() (string, float64) {
	datos, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return "N/D", 0
	}
	var totalKB, disponibleKB uint64
	for _, linea := range strings.Split(string(datos), "\n") {
		if strings.HasPrefix(linea, "MemTotal:") {
			campos := strings.Fields(linea)
			if len(campos) >= 2 {
				totalKB, _ = strconv.ParseUint(campos[1], 10, 64)
			}
		} else if strings.HasPrefix(linea, "MemAvailable:") {
			campos := strings.Fields(linea)
			if len(campos) >= 2 {
				disponibleKB, _ = strconv.ParseUint(campos[1], 10, 64)
			}
		}
	}
	if totalKB == 0 {
		return "N/D", 0
	}
	usadaMB := (totalKB - disponibleKB) / 1024
	totalMB := totalKB / 1024
	porcentaje := float64(totalKB-disponibleKB) * 100.0 / float64(totalKB)
	return fmt.Sprintf("%.0f%% (%dMB/%dMB)", porcentaje, usadaMB, totalMB), porcentaje
}

func contarProcesos() int {
	entradas, err := os.ReadDir("/proc")
	if err != nil {
		return 0
	}
	conteo := 0
	for _, entrada := range entradas {
		if entrada.IsDir() {
			if _, err := strconv.Atoi(entrada.Name()); err == nil {
				conteo++
			}
		}
	}
	return conteo
}

func leerDisco() (string, float64) {
	salida, err := exec.Command("df", "-h", "/").CombinedOutput()
	if err != nil {
		return "N/D", 0
	}
	lineas := strings.Split(strings.TrimSpace(string(salida)), "\n")
	if len(lineas) >= 2 {
		campos := strings.Fields(lineas[1])
		if len(campos) >= 5 {
			textoPorcentaje := campos[4]
			sinSigno := strings.TrimSuffix(textoPorcentaje, "%")
			valor, err := strconv.ParseFloat(sinSigno, 64)
			if err != nil {
				valor = 0
			}
			return textoPorcentaje, valor
		}
	}
	return "N/D", 0
}
