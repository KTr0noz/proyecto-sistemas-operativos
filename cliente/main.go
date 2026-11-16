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

	enviador := json.NewEncoder(conexion)
	lector := json.NewDecoder(conexion)

	fmt.Println("==================================================")
	fmt.Println("          CLIENTE SISTEMAS OPERATIVOS             ")
	fmt.Printf("Conectado a %s:%s | Periodo de reporte: %ds\n", ip, puerto, periodo)
	fmt.Println("==================================================")

	if !autenticar(enviador, lector, periodo) {
		return
	}

	fmt.Println("\n[+] LOGIN EXITOSO. Abriendo ventana OperGUI...")
	iniciarGUI(enviador, lector)
}

func autenticar(enviador *json.Encoder, lector *json.Decoder, periodo int) bool {
	entrada := bufio.NewReader(os.Stdin)

	for {
		fmt.Print("Usuario: ")
		usuario, _ := entrada.ReadString('\n')
		usuario = strings.TrimSpace(usuario)

		fmt.Print("Password: ")
		passwordBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			fmt.Printf("Error leyendo password: %v\n", err)
			return false
		}

		err = enviador.Encode(Mensaje{
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
		if err := lector.Decode(&respuesta); err != nil {
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

func iniciarGUI(enviador *json.Encoder, lector *json.Decoder) {
	programa := app.New()
	ventana := programa.NewWindow("Proyecto Operativos - Ramos")

	cajaComando := widget.NewEntry()
	cajaComando.SetPlaceHolder("Digite el comando UNIX (o 'bye' para salir)")

	textoSalida := widget.NewLabel("Esperando comando...")
	panelSalida := container.NewScroll(textoSalida)
	panelSalida.SetMinSize(fyne.NewSize(500, 180))

	textoReporte := widget.NewLabel("Esperando reporte de recursos...")

	barraCPU := widget.NewProgressBar()
	barraCPU.Max = 100

	barraRAM := widget.NewProgressBar()
	barraRAM.Max = 100

	barraDisco := widget.NewProgressBar()
	barraDisco.Max = 100

	enviarComando := func(texto string) {
		comando := strings.TrimSpace(texto)
		if comando == "" {
			return
		}

		if err := enviador.Encode(Mensaje{Tipo: "comando", Contenido: comando}); err != nil {
			textoSalida.SetText("Error enviando comando al servidor.")
			return
		}

		cajaComando.SetText("")
		if strings.EqualFold(comando, "bye") {
			time.Sleep(300 * time.Millisecond)
			ventana.Close()
		}
	}

	botonEjecutar := widget.NewButton("Ejecutar", func() {
		enviarComando(cajaComando.Text)
	})
	cajaComando.OnSubmitted = enviarComando

	ventana.SetContent(container.NewVBox(
		cajaComando,
		botonEjecutar,
		widget.NewLabel("Salida del comando:"),
		panelSalida,
		widget.NewSeparator(),
		textoReporte,
		widget.NewLabel("CPU (%):"),
		barraCPU,
		widget.NewLabel("RAM (%):"),
		barraRAM,
		widget.NewLabel("Disco (%):"),
		barraDisco,
	))

	ventana.Resize(fyne.NewSize(560, 460))

	go recibirMensajes(
		lector,
		ventana,
		textoSalida,
		textoReporte,
		barraCPU,
		barraRAM,
		barraDisco,
	)

	ventana.ShowAndRun()
}

func recibirMensajes(
	lector *json.Decoder,
	ventana fyne.Window,
	textoSalida *widget.Label,
	textoReporte *widget.Label,
	barraCPU *widget.ProgressBar,
	barraRAM *widget.ProgressBar,
	barraDisco *widget.ProgressBar,
) {
	for {
		var mensaje Mensaje
		if err := lector.Decode(&mensaje); err != nil {
			fyne.Do(func() {
				textoReporte.SetText("Conexion cerrada por el servidor.")
			})
			return
		}

		switch mensaje.Tipo {
		case "reporte":
			fyne.Do(func() {
				textoReporte.SetText(mensaje.Contenido)
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
				textoSalida.SetText(salida)
			})
		case "bye":
			fyne.Do(func() {
				ventana.Close()
			})
			return
		}
	}
}
