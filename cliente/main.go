package main

import (
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
	programa := app.New()
	pedirParametros(programa)
	programa.Run()
}

// Ventana 1: pide IP, puerto y periodo de reporte, y se conecta al servidor.
func pedirParametros(programa fyne.App) {
	ventana := programa.NewWindow("Proyecto Operativos - Conexion")

	cajaIP := widget.NewEntry()
	cajaIP.SetPlaceHolder("Ejemplo: 127.0.0.1")

	cajaPuerto := widget.NewEntry()
	cajaPuerto.SetPlaceHolder("Ejemplo: 2306")

	cajaPeriodo := widget.NewEntry()
	cajaPeriodo.SetPlaceHolder("Ejemplo: 5")

	// Si se ejecuta como: clienteOperativos <IP> <Puerto> <PeriodoReporte>, se llenan los campos.
	if len(os.Args) == 4 {
		cajaIP.SetText(os.Args[1])
		cajaPuerto.SetText(os.Args[2])
		cajaPeriodo.SetText(os.Args[3])
	}

	textoEstado := widget.NewLabel("")
	textoEstado.Wrapping = fyne.TextWrapWord

	botonConectar := widget.NewButton("Conectar", nil)
	botonConectar.OnTapped = func() {
		if botonConectar.Disabled() {
			return
		}

		ip := strings.TrimSpace(cajaIP.Text)
		puerto := strings.TrimSpace(cajaPuerto.Text)
		periodo, err := strconv.Atoi(strings.TrimSpace(cajaPeriodo.Text))
		if err != nil || periodo <= 0 || periodo > 3600 {
			textoEstado.SetText("Error: PeriodoReporte debe ser un numero entero entre 1 y 3600.")
			return
		}

		botonConectar.Disable()
		textoEstado.SetText("Conectando a " + ip + ":" + puerto + "...")

		// La conexion se hace en una goroutine para que la ventana no se congele.
		go func() {
			conexion, err := net.DialTimeout("tcp", ip+":"+puerto, 5*time.Second)
			fyne.Do(func() {
				if err != nil {
					textoEstado.SetText(fmt.Sprintf("Error conectando a %s:%s -> %v", ip, puerto, err))
					botonConectar.Enable()
					return
				}
				pedirLogin(programa, json.NewEncoder(conexion), json.NewDecoder(conexion), periodo)
				ventana.Close()
			})
		}()
	}
	cajaPeriodo.OnSubmitted = func(string) { botonConectar.OnTapped() }

	ventana.SetContent(container.NewVBox(
		widget.NewLabel("IP del servidor:"),
		cajaIP,
		widget.NewLabel("Puerto:"),
		cajaPuerto,
		widget.NewLabel("Periodo de reporte (segundos):"),
		cajaPeriodo,
		botonConectar,
		textoEstado,
	))

	ventana.Resize(fyne.NewSize(400, 0))
	ventana.Show()
}

// Ventana 2: pide usuario y contrasena y los valida con el servidor.
func pedirLogin(programa fyne.App, enviador *json.Encoder, lector *json.Decoder, periodo int) {
	ventana := programa.NewWindow("Proyecto Operativos - Login")

	cajaUsuario := widget.NewEntry()
	cajaUsuario.SetPlaceHolder("Usuario")

	cajaPassword := widget.NewPasswordEntry()
	cajaPassword.SetPlaceHolder("Contrasena")

	textoEstado := widget.NewLabel("")
	textoEstado.Wrapping = fyne.TextWrapWord

	botonIngresar := widget.NewButton("Ingresar", nil)
	botonIngresar.OnTapped = func() {
		if botonIngresar.Disabled() {
			return
		}

		usuario := strings.TrimSpace(cajaUsuario.Text)
		password := cajaPassword.Text
		botonIngresar.Disable()

		// La espera de la respuesta del servidor tambien va en una goroutine.
		go func() {
			respuesta, err := autenticar(enviador, lector, usuario, password, periodo)
			fyne.Do(func() {
				if err != nil {
					textoEstado.SetText("Conexion terminada por el servidor.")
					return
				}

				if respuesta.Tipo == "auth_ok" {
					iniciarGUI(programa, enviador, lector)
					ventana.Close()
					return
				}

				if respuesta.IntentosRestantes <= 0 {
					textoEstado.SetText(respuesta.Contenido + "\nAcceso bloqueado. Terminando conexion.")
					return
				}

				textoEstado.SetText(fmt.Sprintf("%s\nReintentos disponibles: %d", respuesta.Contenido, respuesta.IntentosRestantes))
				cajaPassword.SetText("")
				botonIngresar.Enable()
			})
		}()
	}
	cajaPassword.OnSubmitted = func(string) { botonIngresar.OnTapped() }

	ventana.SetContent(container.NewVBox(
		widget.NewLabel("Usuario:"),
		cajaUsuario,
		widget.NewLabel("Contrasena:"),
		cajaPassword,
		botonIngresar,
		textoEstado,
	))

	ventana.Resize(fyne.NewSize(400, 0))
	ventana.Show()
}

// Envia las credenciales al servidor y devuelve su respuesta (auth_ok o auth_error).
func autenticar(enviador *json.Encoder, lector *json.Decoder, usuario, password string, periodo int) (Mensaje, error) {
	var respuesta Mensaje

	err := enviador.Encode(Mensaje{
		Tipo:     "auth",
		Usuario:  usuario,
		Password: password,
		Periodo:  periodo,
	})
	if err != nil {
		return respuesta, err
	}

	err = lector.Decode(&respuesta)
	return respuesta, err
}

// Ventana 3: la ventana principal OperGUI (comandos + reportes con 3 barras).
func iniciarGUI(programa fyne.App, enviador *json.Encoder, lector *json.Decoder) {
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

	ventana.Show()
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
