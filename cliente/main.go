package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"golang.org/x/term"
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

func main() {
	if len(os.Args) != 4 {
		fmt.Println("Uso: clienteOperativos <IP> <Puerto> <PeriodoReporte>")
		fmt.Println("Ejemplo: clienteOperativos 127.0.0.1 2306 5")
		return
	}

	ip := os.Args[1]
	puerto := os.Args[2]
	periodo, err := strconv.Atoi(os.Args[3])
	if err != nil || periodo <= 0 {
		fmt.Println("Error: PeriodoReporte debe ser un numero entero positivo mayor a 0.")
		return
	}

	conexion, err := net.Dial("tcp", ip+":"+puerto)
	if err != nil {
		fmt.Printf("Error conectando a %s:%s -> %v\n", ip, puerto, err)
		return
	}
	defer conexion.Close()

	codificador := json.NewEncoder(conexion)
	decodificador := json.NewDecoder(conexion)

	fmt.Println("==================================================")
	fmt.Println("          CLIENTE SISTEMAS OPERATIVOS             ")
	fmt.Printf("Conectado a %s:%s | Periodo de reporte: %ds\n", ip, puerto, periodo)
	fmt.Println("==================================================")

	if !autenticarEnConsola(codificador, decodificador, periodo) {
		return
	}

	fmt.Println("\n[+] LOGIN EXITOSO. Abriendo ventana OperGUI...")
	iniciarVentanaOperGUI(codificador, decodificador)
}

func autenticarEnConsola(codificador *json.Encoder, decodificador *json.Decoder, periodo int) bool {
	lector := bufio.NewReader(os.Stdin)

	for {
		fmt.Print("Usuario: ")
		usuario, _ := lector.ReadString('\n')
		usuario = strings.TrimSpace(usuario)

		fmt.Print("Password: ")
		passwordBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			fmt.Printf("Error leyendo password: %v\n", err)
			return false
		}

		err = codificador.Encode(Mensaje{
			Tipo:     "auth",
			Usuario:  usuario,
			Password: string(passwordBytes),
			Periodo:  periodo,
		})
		if err != nil {
			fmt.Println("Error enviando credenciales al servidor.")
			return false
		}

		var respuesta Mensaje
		if err := decodificador.Decode(&respuesta); err != nil {
			fmt.Println("Conexion terminada por el servidor.")
			return false
		}

		if respuesta.Tipo == "auth_ok" {
			return true
		}

		fmt.Printf("[-] %s\n", respuesta.Contenido)
		if respuesta.IntentosRestantes <= 0 {
			fmt.Println("Acceso bloqueado. Terminando conexion.")
			return false
		}
		fmt.Printf("Reintentos disponibles: %d\n\n", respuesta.IntentosRestantes)
	}
}

func iniciarVentanaOperGUI(codificador *json.Encoder, decodificador *json.Decoder) {
	aplicacion := app.New()
	ventana := aplicacion.NewWindow("Proyecto Operativos - Ramos")

	cajaComando := widget.NewEntry()
	cajaComando.SetPlaceHolder("Digite el comando UNIX (o 'bye' para salir)")

	etiquetaResultado := widget.NewLabel("Esperando comando...")
	contenedorResultado := container.NewScroll(etiquetaResultado)
	contenedorResultado.SetMinSize(fyne.NewSize(500, 180))

	etiquetaReporte := widget.NewLabel("Esperando reporte de recursos...")

	barraCPU := widget.NewProgressBar()
	barraCPU.Max = 100

	barraRAM := widget.NewProgressBar()
	barraRAM.Max = 100

	barraDisco := widget.NewProgressBar()
	barraDisco.Max = 100

	ejecutarComando := func(texto string) {
		comando := strings.TrimSpace(texto)
		if comando == "" {
			return
		}

		if err := codificador.Encode(Mensaje{Tipo: "comando", Contenido: comando}); err != nil {
			etiquetaResultado.SetText("Error enviando comando al servidor.")
			return
		}

		cajaComando.SetText("")
		if strings.EqualFold(comando, "bye") {
			time.Sleep(300 * time.Millisecond)
			ventana.Close()
		}
	}

	botonEjecutar := widget.NewButton("Ejecutar", func() {
		ejecutarComando(cajaComando.Text)
	})
	cajaComando.OnSubmitted = ejecutarComando

	ventana.SetContent(container.NewVBox(
		cajaComando,
		botonEjecutar,
		widget.NewLabel("Salida del comando:"),
		contenedorResultado,
		widget.NewSeparator(),
		etiquetaReporte,
		widget.NewLabel("CPU (%):"),
		barraCPU,
		widget.NewLabel("RAM (%):"),
		barraRAM,
		widget.NewLabel("Disco (%):"),
		barraDisco,
	))

	ventana.Resize(fyne.NewSize(560, 460))

	go recibirReportesYRespuestas(
		decodificador,
		ventana,
		etiquetaResultado,
		etiquetaReporte,
		barraCPU,
		barraRAM,
		barraDisco,
	)

	ventana.ShowAndRun()
}

func recibirReportesYRespuestas(
	decodificador *json.Decoder,
	ventana fyne.Window,
	etiquetaResultado *widget.Label,
	etiquetaReporte *widget.Label,
	barraCPU *widget.ProgressBar,
	barraRAM *widget.ProgressBar,
	barraDisco *widget.ProgressBar,
) {
	for {
		var mensaje Mensaje
		if err := decodificador.Decode(&mensaje); err != nil {
			fyne.Do(func() {
				etiquetaReporte.SetText("Conexion cerrada por el servidor.")
			})
			return
		}

		switch mensaje.Tipo {
		case "reporte":
			fyne.Do(func() {
				etiquetaReporte.SetText(mensaje.Contenido)
				barraCPU.SetValue(mensaje.PorcentajeCPU)
				barraRAM.SetValue(mensaje.PorcentajeRAM)
				barraDisco.SetValue(mensaje.PorcentajeDisco)
			})
		case "salida":
			salida := mensaje.Contenido
			if salida == "" {
				salida = "(comando ejecutado sin salida)"
			}
			fyne.Do(func() {
				etiquetaResultado.SetText(salida)
			})
		case "bye":
			fyne.Do(func() {
				ventana.Close()
			})
			return
		}
	}
}
