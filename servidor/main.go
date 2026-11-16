package main

import (
	"crypto/sha256"
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
	config, err := leerConfiguracion("servidor.conf")
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

func enviar(conexion net.Conn, mensaje Mensaje) error {
	return json.NewEncoder(conexion).Encode(mensaje)
}

func atenderCliente(conexion net.Conn, config Configuracion) {
	defer conexion.Close()

	ipCliente, _, _ := net.SplitHostPort(conexion.RemoteAddr().String())
	fmt.Printf("[+] Nueva conexion desde: %s\n", ipCliente)

	if config.IPPermitida != "*" && config.IPPermitida != ipCliente {
		fmt.Printf("[-] Conexion rechazada: IP %s no autorizada en servidor.conf\n", ipCliente)
		return
	}

	lector := json.NewDecoder(conexion)
	usuario, periodo, ok := autenticar(conexion, lector, config, ipCliente)
	if !ok {
		return
	}

	go enviarReportes(conexion, periodo)

	for {
		var mensaje Mensaje
		if err := lector.Decode(&mensaje); err != nil {
			break
		}

		if mensaje.Tipo != "comando" {
			continue
		}

		comando := strings.TrimSpace(mensaje.Contenido)
		if strings.EqualFold(comando, "bye") {
			fmt.Printf("[*] Usuario '%s' envio 'bye'. Cerrando sesion.\n", usuario)
			enviar(conexion, Mensaje{Tipo: "bye", Contenido: "Sesion finalizada."})
			break
		}

		fmt.Printf("[*] Ejecutando comando de '%s': %s\n", usuario, comando)
		salida, err := exec.Command("bash", "-c", comando).CombinedOutput()
		if err != nil && len(salida) == 0 {
			salida = []byte(fmt.Sprintf("Error ejecutando: %v\n", err))
		}

		if err := enviar(conexion, Mensaje{Tipo: "salida", Contenido: string(salida)}); err != nil {
			break
		}
	}
}

func autenticar(conexion net.Conn, lector *json.Decoder, config Configuracion, ipCliente string) (string, int, bool) {
	intentos := config.IntentosFallidos

	for intentos > 0 {
		var mensaje Mensaje
		if err := lector.Decode(&mensaje); err != nil {
			return "", 0, false
		}

		if mensaje.Tipo != "auth" {
			enviar(conexion, Mensaje{Tipo: "auth_error", Contenido: "Protocolo invalido: se requiere autenticacion"})
			return "", 0, false
		}

		if validarUsuario(mensaje.Usuario, mensaje.Password) && usuarioPermitido(mensaje.Usuario, config.UsuariosPermitidos) {
			periodo := mensaje.Periodo
			if periodo <= 0 {
				periodo = 5
			}
			enviar(conexion, Mensaje{Tipo: "auth_ok", Contenido: "Autenticacion exitosa"})
			fmt.Printf("[+] Usuario '%s' autenticado exitosamente desde %s\n", mensaje.Usuario, ipCliente)
			return mensaje.Usuario, periodo, true
		}

		intentos--
		fmt.Printf("[-] Intento fallido para '%s'. Quedan %d intentos.\n", mensaje.Usuario, intentos)

		if intentos <= 0 {
			enviar(conexion, Mensaje{
				Tipo:              "auth_error",
				Contenido:         "Limite de intentos fallidos superado. Conexion cerrada.",
				IntentosRestantes: 0,
			})
			return "", 0, false
		}

		enviar(conexion, Mensaje{
			Tipo:              "auth_error",
			Contenido:         "Usuario o contrasena incorrectos.",
			IntentosRestantes: intentos,
		})
	}

	return "", 0, false
}

func enviarReportes(conexion net.Conn, periodo int) {
	numero := 0
	for {
		time.Sleep(time.Duration(periodo) * time.Second)
		numero++
		resumen, pctCPU, pctRAM, pctDisco := obtenerReporte()
		texto := fmt.Sprintf("Reporte #%d | %s", numero, resumen)
		err := enviar(conexion, Mensaje{
			Tipo:            "reporte",
			Contenido:       texto,
			PorcentajeCPU:   pctCPU,
			PorcentajeRAM:   pctRAM,
			PorcentajeDisco: pctDisco,
		})
		if err != nil {
			return
		}
	}
}

func leerConfiguracion(ruta string) (Configuracion, error) {
	config := Configuracion{
		IPPermitida:      "*",
		Puerto:           "2306",
		IntentosFallidos: 3,
	}

	datos, err := os.ReadFile(ruta)
	if err != nil {
		return config, err
	}

	lineas := strings.Split(string(datos), "\n")
	for _, linea := range lineas {
		linea = strings.TrimSpace(linea)
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
			if n, err := strconv.Atoi(valor); err == nil && n > 0 {
				config.IntentosFallidos = n
			}
		case "usuarios_permitidos":
			for _, u := range strings.Split(valor, ",") {
				u = strings.TrimSpace(u)
				if u != "" {
					config.UsuariosPermitidos = append(config.UsuariosPermitidos, u)
				}
			}
		}
	}
	return config, nil
}

func validarUsuario(usuario, password string) bool {
	datos, err := os.ReadFile("usuarios.db")
	if err != nil {
		return false
	}

	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(password)))

	lineas := strings.Split(string(datos), "\n")
	for _, linea := range lineas {
		linea = strings.TrimSpace(linea)
		if linea == "" || strings.HasPrefix(linea, "#") {
			continue
		}
		partes := strings.Split(linea, ":")
		if len(partes) >= 2 && partes[0] == usuario {
			return partes[1] == hash
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

func obtenerReporte() (string, float64, float64, float64) {
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
	campos := strings.Fields(lineas[0])
	if len(campos) < 5 || campos[0] != "cpu" {
		return "N/D", 0
	}

	total := 0
	for _, valor := range campos[1:] {
		n, _ := strconv.Atoi(valor)
		total += n
	}
	inactivo, _ := strconv.Atoi(campos[4])
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

	var totalKB, disponibleKB int
	for _, linea := range strings.Split(string(datos), "\n") {
		campos := strings.Fields(linea)
		if len(campos) < 2 {
			continue
		}
		if campos[0] == "MemTotal:" {
			totalKB, _ = strconv.Atoi(campos[1])
		} else if campos[0] == "MemAvailable:" {
			disponibleKB, _ = strconv.Atoi(campos[1])
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
	salida, err := exec.Command("df", "/").CombinedOutput()
	if err != nil {
		return "N/D", 0
	}

	lineas := strings.Split(strings.TrimSpace(string(salida)), "\n")
	if len(lineas) < 2 {
		return "N/D", 0
	}

	campos := strings.Fields(lineas[1])
	if len(campos) < 5 {
		return "N/D", 0
	}

	textoPorcentaje := campos[4]
	sinSigno := strings.TrimSuffix(textoPorcentaje, "%")
	valor, err := strconv.Atoi(sinSigno)
	if err != nil {
		return textoPorcentaje, 0
	}
	return textoPorcentaje, float64(valor)
}
