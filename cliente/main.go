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

	"golang.org/x/term"
)

type Mensaje struct {
	Tipo              string `json:"tipo"`
	Contenido         string `json:"contenido,omitempty"`
	Usuario           string `json:"usuario,omitempty"`
	Password          string `json:"password,omitempty"`
	Periodo           int    `json:"periodo,omitempty"`
	IntentosRestantes int    `json:"intentos_restantes,omitempty"`
}

func main() {
	// 1. Validar parametros recibidos: clienteOperativos <IP> <Puerto> <PeriodoReporte>
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

	// Conectar al socket Servidor TCP
	conn, err := net.Dial("tcp", ip+":"+puerto)
	if err != nil {
		fmt.Printf("Error conectando a %s:%s -> %v\n", ip, puerto, err)
		return
	}
	defer conn.Close()

	encoder := json.NewEncoder(conn)
	decoder := json.NewDecoder(conn)

	// 2. Solicitar credenciales de acceso (Usuario y Password sin eco)
	lector := bufio.NewReader(os.Stdin)
	fmt.Println("==================================================")
	fmt.Println("          CLIENTE SISTEMAS OPERATIVOS             ")
	fmt.Printf("Conectado a %s:%s | Periodo de reporte: %ds\n", ip, puerto, periodo)
	fmt.Println("==================================================")

	var autenticado bool
	for !autenticado {
		fmt.Print("Usuario: ")
		usuario, _ := lector.ReadString('\n')
		usuario = strings.TrimSpace(usuario)

		fmt.Print("Password: ")
		passBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println() // Salto de linea tras password
		if err != nil {
			fmt.Printf("Error leyendo password: %v\n", err)
			return
		}
		password := string(passBytes)

		// Enviar credenciales
		err = encoder.Encode(Mensaje{
			Tipo:     "auth",
			Usuario:  usuario,
			Password: password,
			Periodo:  periodo,
		})
		if err != nil {
			fmt.Println("Error enviando credenciales al servidor.")
			return
		}

		// Recibir respuesta del servidor
		var resp Mensaje
		if err := decoder.Decode(&resp); err != nil {
			fmt.Println("Conexion terminada por el servidor.")
			return
		}

		if resp.Tipo == "auth_ok" {
			autenticado = true
			fmt.Println("\n[+] LOGIN EXITOSO. Bienvenido al sistema remoto.")
			fmt.Println("[*] Ingrese comandos UNIX para ejecutar, o 'bye' para salir.\n")
		} else {
			fmt.Printf("[-] %s\n", resp.Contenido)
			if resp.IntentosRestantes <= 0 {
				fmt.Println("Acceso bloqueado. Terminando conexion.")
				return
			}
			fmt.Printf("Reintentos disponibles: %d\n\n", resp.IntentosRestantes)
		}
	}

	// 3. Ejecucion en paralelo
	// Goroutine 1: Recibe reporte de consumo cada n segundos y la salida de comandos
	canalSalida := make(chan struct{})

	go func() {
		for {
			var msg Mensaje
			if err := decoder.Decode(&msg); err != nil {
				fmt.Println("\n[Conexion cerrada por el servidor]")
				close(canalSalida)
				return
			}

			switch msg.Tipo {
			case "reporte":
				// Presentacion del reporte de consumo recibido cada n segundos
				fmt.Printf("\n>>> [%s]\noper%% ", msg.Contenido)
			case "salida":
				// Salida de la ejecucion remota del comando UNIX
				fmt.Print(msg.Contenido)
				fmt.Print("oper% ")
			case "bye":
				fmt.Println("El servidor confirmo la orden de salida.")
				close(canalSalida)
				return
			}
		}
	}()

	// Goroutine 2 / Bucle Principal: Lectura y envio de comandos UNIX remotos
	fmt.Print("oper% ")
	for {
		select {
		case <-canalSalida:
			return
		default:
			linea, err := lector.ReadString('\n')
			if err != nil {
				return
			}
			linea = strings.TrimSpace(linea)
			if linea == "" {
				fmt.Print("oper% ")
				continue
			}

			// Enviar comando al servidor remoto
			encoder.Encode(Mensaje{
				Tipo:      "comando",
				Contenido: linea,
			})

			// Verificacion de orden de salida
			if strings.EqualFold(linea, "bye") {
				time.Sleep(500 * time.Millisecond)
				return
			}
		}
	}
}
