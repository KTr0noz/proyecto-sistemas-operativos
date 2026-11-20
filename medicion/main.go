package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Herramienta de medicion para el paper: se conecta al servidor igual que el cliente,
// mide cuanto tarda cada comando en volver (envio -> salida) y saca los promedios.
// Uso: medirLatencia <IP> <Puerto> <Usuario> <Password> [Repeticiones]

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
	Directorio        string  `json:"directorio,omitempty"`
}

type Categoria struct {
	Nombre   string
	Comandos []string
}

var categorias = []Categoria{
	{"Comandos simples", []string{
		"pwd", "whoami", "date", "hostname", "uptime",
	}},
	{"Comandos con parametros", []string{
		"ls -la /etc", "uname -a", "df -h /", "free -m", "ps -e -o pid,comm",
	}},
	{"Comandos con tuberias", []string{
		"ps -e | wc -l",
		"cat /etc/passwd | cut -d: -f1 | sort | head -5",
		"ls -la /usr/bin | sort -k5 -n | tail -3",
		"cat /proc/cpuinfo | grep processor | wc -l",
		"ps aux | sort -nrk 3 | head -5 | awk '{print $2, $3, $11}'",
	}},
	{"Comandos dobles con &&", []string{
		"cd /tmp && pwd", "uname -r && hostname", "date && uptime", "whoami && id", "test -d /etc && echo existe",
	}},
	{"Consulta de CPU en uso", []string{
		"top -bn1 | head -5", "cat /proc/loadavg", "grep 'cpu ' /proc/stat", "ps -eo pcpu,comm --sort=-pcpu | head -5", "nproc",
	}},
}

func main() {
	if len(os.Args) < 5 {
		fmt.Println("Uso: medirLatencia <IP> <Puerto> <Usuario> <Password> [Repeticiones]")
		return
	}
	direccion := os.Args[1] + ":" + os.Args[2]
	usuario := os.Args[3]
	password := os.Args[4]
	repeticiones := 20
	if len(os.Args) > 5 {
		if n, err := strconv.Atoi(os.Args[5]); err == nil && n > 0 {
			repeticiones = n
		}
	}

	fmt.Println("=== Medicion de latencia - Proyecto Operativos ===")
	fmt.Printf("Servidor: %s | Cliente: %s/%s (%d hilos) | Repeticiones: %d\n",
		direccion, runtime.GOOS, runtime.GOARCH, runtime.NumCPU(), repeticiones)
	fmt.Println("Fecha:", time.Now().Format("2006-01-02 15:04:05"))

	// 1) Conexion TCP y autenticacion: una conexion nueva en cada repeticion.
	var tiemposTCP, tiemposAuth []time.Duration
	for i := 0; i < repeticiones; i++ {
		inicio := time.Now()
		conexion, err := net.DialTimeout("tcp", direccion, 5*time.Second)
		if err != nil {
			fmt.Println("Error conectando:", err)
			return
		}
		tiemposTCP = append(tiemposTCP, time.Since(inicio))

		enviador, lector := json.NewEncoder(conexion), json.NewDecoder(conexion)
		if err := autenticar(enviador, lector, usuario, password, 3600); err != nil {
			fmt.Println("Error autenticando:", err)
			conexion.Close()
			return
		}
		tiemposAuth = append(tiemposAuth, time.Since(inicio))

		enviador.Encode(Mensaje{Tipo: "comando", Contenido: "bye"})
		conexion.Close()
	}

	// 2) Sesion principal (periodo 3600 s para que los reportes no se mezclen con la medicion).
	conexion, err := net.DialTimeout("tcp", direccion, 5*time.Second)
	if err != nil {
		fmt.Println("Error conectando:", err)
		return
	}
	defer conexion.Close()
	enviador, lector := json.NewEncoder(conexion), json.NewDecoder(conexion)
	if err := autenticar(enviador, lector, usuario, password, 3600); err != nil {
		fmt.Println("Error autenticando:", err)
		return
	}

	fmt.Println("\n[Servidor]")
	for _, comando := range []string{
		"grep PRETTY_NAME /etc/os-release",
		"uname -srm",
		"nproc",
		"grep MemTotal /proc/meminfo",
		"grep -m1 'model name' /proc/cpuinfo",
	} {
		_, salida, _ := ejecutar(enviador, lector, comando)
		fmt.Println("  " + strings.TrimSpace(salida))
	}

	fmt.Printf("\n[Conexion y autenticacion] (%d conexiones)\n", repeticiones)
	imprimir("Conexion TCP", tiemposTCP)
	imprimir("Conexion + autenticacion SHA-256", tiemposAuth)

	// Linea base: un comando de la lista negra vuelve sin ejecutar bash (solo red + protocolo).
	fmt.Println("\n[Linea base: comando no permitido, no se ejecuta bash]")
	imprimir("sudo -n true", medir(enviador, lector, "sudo -n true", repeticiones))

	var todos []time.Duration
	var promediosCategoria []string
	for _, categoria := range categorias {
		fmt.Printf("\n[%s]\n", categoria.Nombre)
		var deCategoria []time.Duration
		for _, comando := range categoria.Comandos {
			ejecutar(enviador, lector, comando) // calentamiento, no se cuenta
			tiempos := medir(enviador, lector, comando, repeticiones)
			imprimir(comando, tiempos)
			deCategoria = append(deCategoria, tiempos...)
		}
		promedio, _, _ := estadisticas(deCategoria)
		fmt.Printf("  %-58s %8.2f ms\n", "Promedio de la categoria", promedio)
		promediosCategoria = append(promediosCategoria, fmt.Sprintf("  %-34s %8.2f ms", categoria.Nombre, promedio))
		todos = append(todos, deCategoria...)
	}

	_, rss, _ := ejecutar(enviador, lector, "ps -o rss= -C servidorOperativos | head -1")
	enviador.Encode(Mensaje{Tipo: "comando", Contenido: "bye"})

	// 3) Reporte periodico: con periodo de 1 s se mide el tiempo entre reportes.
	// Lo que pasa de 1 s es lo que tarda el servidor en medir la CPU y enviar el reporte.
	const reportes = 10
	conexionReporte, err := net.DialTimeout("tcp", direccion, 5*time.Second)
	if err != nil {
		fmt.Println("Error conectando:", err)
		return
	}
	defer conexionReporte.Close()
	enviadorR, lectorR := json.NewEncoder(conexionReporte), json.NewDecoder(conexionReporte)
	if err := autenticar(enviadorR, lectorR, usuario, password, 1); err != nil {
		fmt.Println("Error autenticando:", err)
		return
	}
	var llegadas []time.Time
	for len(llegadas) < reportes {
		var mensaje Mensaje
		if err := lectorR.Decode(&mensaje); err != nil {
			fmt.Println("Error leyendo reporte:", err)
			return
		}
		if mensaje.Tipo == "reporte" {
			llegadas = append(llegadas, time.Now())
		}
	}
	enviadorR.Encode(Mensaje{Tipo: "comando", Contenido: "bye"})

	var intervalos, retardos []time.Duration
	for i := 1; i < len(llegadas); i++ {
		intervalo := llegadas[i].Sub(llegadas[i-1])
		intervalos = append(intervalos, intervalo)
		retardos = append(retardos, intervalo-time.Second)
	}

	fmt.Printf("\n[Reporte periodico de CPU, RAM, procesos y disco] (periodo 1 s, %d reportes)\n", reportes)
	imprimir("Intervalo entre reportes", intervalos)
	imprimir("Retardo del reporte (intervalo - periodo)", retardos)

	fmt.Println("\n[Resumen por categoria]")
	for _, linea := range promediosCategoria {
		fmt.Println(linea)
	}
	promedio, minimo, maximo := estadisticas(todos)
	fmt.Printf("\nPromedio general (%d comandos x %d repeticiones): %.2f ms (min %.2f, max %.2f)\n",
		len(todos)/repeticiones, repeticiones, promedio, minimo, maximo)
	fmt.Printf("Memoria residente del servidor (RSS): %s KB\n", strings.TrimSpace(rss))
}

func autenticar(enviador *json.Encoder, lector *json.Decoder, usuario, password string, periodo int) error {
	// El servidor saluda primero al aceptar la conexion.
	var saludo Mensaje
	if err := lector.Decode(&saludo); err != nil {
		return err
	}
	if saludo.Tipo != "bienvenida" {
		return fmt.Errorf("%s", saludo.Contenido)
	}

	err := enviador.Encode(Mensaje{Tipo: "auth", Usuario: usuario, Password: password, Periodo: periodo})
	if err != nil {
		return err
	}
	var respuesta Mensaje
	if err := lector.Decode(&respuesta); err != nil {
		return err
	}
	if respuesta.Tipo != "auth_ok" {
		return fmt.Errorf("%s", respuesta.Contenido)
	}
	return nil
}

// Envia un comando y espera su salida. Los reportes que lleguen en medio se ignoran.
func ejecutar(enviador *json.Encoder, lector *json.Decoder, comando string) (time.Duration, string, error) {
	inicio := time.Now()
	if err := enviador.Encode(Mensaje{Tipo: "comando", Contenido: comando}); err != nil {
		return 0, "", err
	}
	for {
		var mensaje Mensaje
		if err := lector.Decode(&mensaje); err != nil {
			return 0, "", err
		}
		if mensaje.Tipo == "salida" {
			return time.Since(inicio), mensaje.Contenido, nil
		}
	}
}

func medir(enviador *json.Encoder, lector *json.Decoder, comando string, repeticiones int) []time.Duration {
	var tiempos []time.Duration
	for i := 0; i < repeticiones; i++ {
		tiempo, _, err := ejecutar(enviador, lector, comando)
		if err != nil {
			fmt.Println("Error ejecutando:", err)
			os.Exit(1)
		}
		tiempos = append(tiempos, tiempo)
	}
	return tiempos
}

// Devuelve promedio, minimo y maximo en milisegundos.
func estadisticas(tiempos []time.Duration) (float64, float64, float64) {
	if len(tiempos) == 0 {
		return 0, 0, 0
	}
	suma, minimo, maximo := 0.0, 0.0, 0.0
	for i, tiempo := range tiempos {
		ms := float64(tiempo.Microseconds()) / 1000
		suma += ms
		if i == 0 || ms < minimo {
			minimo = ms
		}
		if ms > maximo {
			maximo = ms
		}
	}
	return suma / float64(len(tiempos)), minimo, maximo
}

func imprimir(nombre string, tiempos []time.Duration) {
	promedio, minimo, maximo := estadisticas(tiempos)
	fmt.Printf("  %-58s %8.2f ms  (min %.2f, max %.2f)\n", nombre, promedio, minimo, maximo)
}
