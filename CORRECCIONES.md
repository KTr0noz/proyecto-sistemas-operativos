# Correcciones de errores: antes vs después

Este documento compara el código **antes** (con el error) y **después** (con la solución) de los 5 errores corregidos, y resume las pruebas hechas al servidor y al cliente.

- **Antes:** la rama `old` (proyecto original) y el commit de la GUI de conexión/login, para el código del cliente.
- **Después:** la rama `main` con las correcciones.
- **Tamaño del cambio:** 2 archivos (`servidor/main.go` y `cliente/main.go`), +33 / −17 líneas, una sola función nueva (`leerTiemposCPU`) y un solo import nuevo (`context`). La estructura del programa no cambia.

## Resumen

| # | Error | Archivo | Antes (medido) | Después (medido) |
|---|---|---|---|---|
| 1 | CPU medida desde que arrancó el equipo | servidor | Con 4 núcleos al 100% reportaba **17.8%** | Reporta **100%** |
| 2 | Comandos sin límite de tiempo ni de tamaño | servidor | `ping` dejaba la sesión **colgada para siempre**; una salida de 30 MB se enviaba **completa** | `ping` se detiene a los **10 s** con aviso; la salida se recorta a **64 KB** |
| 3 | Periodo de reporte sin tope (desbordamiento) | servidor + cliente | `periodo=9223372037` producía **453 reportes en 1,5 s** | **0 reportes**; el cliente rechaza valores fuera de 1–3600 |
| 4 | Doble cierre de ventana con `bye` | cliente | La GUI se congelaba **300 ms** y la ventana se cerraba **2 veces** | **0 ms**; la ventana se cierra **1 vez**, cuando el servidor confirma |
| 5 | `Accept` sin pausa cuando falla | servidor | Con los descriptores agotados: **99% de CPU** y **294 335 líneas de log por segundo** | **0% de CPU** y **10 líneas por segundo** |

---

## 1. CPU medida desde que arrancó el equipo (`servidor/main.go`, `leerCPU`)

**Problema:** `/proc/stat` no dice cuánta CPU se está usando *ahora*. Guarda contadores **acumulados desde que se encendió el equipo** (tiempo total y tiempo inactivo). El código leía una sola vez, así que calculaba el **promedio desde el arranque** y la barra de CPU casi no se movía aunque la máquina estuviera al 100%.

**Antes:**
```go
func leerCPU() (string, float64) {
	datos, err := os.ReadFile("/proc/stat")
	// ... toma la primera linea "cpu ..."
	total := 0
	for _, valor := range campos[1:] {
		n, _ := strconv.Atoi(valor)
		total += n
	}
	inactivo, _ := strconv.Atoi(campos[4])
	if total == 0 {
		return "0%", 0
	}

	porcentaje := float64(total-inactivo) * 100.0 / float64(total)   // promedio desde el arranque
	return fmt.Sprintf("%.0f%%", porcentaje), porcentaje
}
```

**Después:** la lectura de `/proc/stat` pasa a `leerTiemposCPU()`, y `leerCPU()` toma **dos muestras separadas 0,5 s y las resta**.
```go
// /proc/stat acumula tiempos desde que arranco el equipo: se toman dos muestras y se restan.
func leerCPU() (string, float64) {
	total1, inactivo1 := leerTiemposCPU()
	time.Sleep(500 * time.Millisecond)
	total2, inactivo2 := leerTiemposCPU()
	if total2 == total1 {
		return "N/D", 0
	}

	porcentaje := float64((total2-total1)-(inactivo2-inactivo1)) * 100.0 / float64(total2-total1)
	return fmt.Sprintf("%.0f%%", porcentaje), porcentaje
}

func leerTiemposCPU() (int, int) {
	datos, err := os.ReadFile("/proc/stat")
	// ... misma lectura de antes: suma todos los campos (total) y toma campos[4] (inactivo)
	return total, inactivo
}
```

**Cómo explicarlo:** "Se toman dos fotos de los contadores con medio segundo de diferencia. Si en ese medio segundo pasaron 200 unidades de tiempo de CPU y 150 fueron inactivas, el uso es (200 − 150) / 200 = 25%."

El `time.Sleep` de 0,5 s no congela nada: corre dentro de la goroutine `enviarReportes`, no en el bucle que atiende comandos.

---

## 2. Comandos sin límite de tiempo ni de tamaño (`servidor/main.go`, `atenderCliente`)

**Problema:**
- Un comando que nunca termina, como `ping google.com` (muy típico en una demo), **colgaba la sesión para siempre**. El servidor esperaba a que terminara y ni siquiera procesaba el `bye`.
- Una salida enorme (`cat` de un log grande, `find /`) se mandaba **completa en un solo mensaje**: subía mucho la memoria del servidor y podía congelar la GUI del cliente.
- Un comando en segundo plano (`sleep 4 &`) hacía esperar al servidor hasta que ese proceso terminara.

**Antes:**
```go
salida, err := exec.Command("bash", "-c", comando).CombinedOutput()
if err != nil && len(salida) == 0 {
	salida = []byte(fmt.Sprintf("Error ejecutando: %v\n", err))
}
```

**Después:**
```go
ctx, cancelar := context.WithTimeout(context.Background(), 10*time.Second)
comandoBash := exec.CommandContext(ctx, "bash", "-c", comando)
comandoBash.WaitDelay = time.Second
salida, err := comandoBash.CombinedOutput()
cancelar()

if ctx.Err() == context.DeadlineExceeded {
	salida = append(salida, "\n[Comando detenido: supero el tiempo limite de 10 segundos]\n"...)
} else if err != nil && len(salida) == 0 {
	salida = []byte(fmt.Sprintf("Error ejecutando: %v\n", err))
}
if len(salida) > 64*1024 {
	salida = append(salida[:64*1024], "\n[Salida recortada a 64 KB]\n"...)
}
```

**Qué hace cada línea:**
- `context.WithTimeout(..., 10*time.Second)` crea un **temporizador de 10 segundos**.
- `exec.CommandContext(ctx, ...)` asocia el comando al temporizador: si vence, Go **mata el proceso** (SIGKILL).
- `WaitDelay = time.Second`: cuando el proceso termina (o lo matan), Go espera como máximo 1 s más por su salida. Así no se queda esperando a procesos que quedaron en segundo plano (`sleep 4 &`).
- `cancelar()` libera el temporizador cuando el comando ya terminó.
- `ctx.Err() == context.DeadlineExceeded` pregunta si **se venció el tiempo**. Se compara con `DeadlineExceeded`, y no simplemente con `!= nil`, porque después de `cancelar()` el contexto siempre tiene error (`Canceled`). Si solo se revisara `!= nil`, **todos** los comandos saldrían como "detenidos"; esto se detectó en las pruebas.
- `salida[:64*1024]` conserva los primeros 64 KB y le agrega el aviso.

**Caso especial:** con `sleep 4 &` la respuesta ahora llega en 1 s con el texto `Error ejecutando: exec: WaitDelay expired before I/O complete`. Significa que bash terminó pero un proceso en segundo plano seguía con la salida abierta, y el servidor dejó de esperarlo.

---

## 3. Periodo de reporte sin tope (`servidor/main.go` y `cliente/main.go`)

**Problema:** el periodo se convierte a duración con `time.Duration(periodo) * time.Second`, y una duración en Go es un entero de 64 bits en nanosegundos. Con un periodo enorme (por ejemplo `9223372037`), la multiplicación **se desborda y da un número negativo**: `time.Sleep` con un valor negativo no espera nada, y el servidor enviaba reportes sin parar (cada uno ejecutando `df`). El cliente aceptaba ese valor.

**Antes (servidor, `autenticar`):**
```go
if periodo <= 0 {
	periodo = 5
}
```
**Después (servidor):**
```go
if periodo <= 0 || periodo > 3600 {
	periodo = 5
}
```

**Antes (cliente, `pedirParametros`):**
```go
if err != nil || periodo <= 0 {
	textoEstado.SetText("Error: PeriodoReporte debe ser un numero entero positivo mayor a 0.")
```
**Después (cliente):**
```go
if err != nil || periodo <= 0 || periodo > 3600 {
	textoEstado.SetText("Error: PeriodoReporte debe ser un numero entero entre 1 y 3600.")
```

**Cómo explicarlo:** "Se valida en los dos lados: el cliente no deja enviar un periodo fuera de 1 a 3600 segundos (1 hora), y el servidor no confía en el cliente y vuelve a validarlo."

---

## 4. Doble cierre de ventana con `bye` (`cliente/main.go`, `iniciarGUI`)

**Problema:** al escribir `bye` la ventana se cerraba **dos veces**:
1. `recibirMensajes` la cerraba al llegar la respuesta `bye` del servidor.
2. `enviarComando` esperaba 300 ms con `time.Sleep` y la cerraba otra vez.

El `time.Sleep` corría en el **hilo de la GUI**, así que la ventana quedaba congelada 300 ms.

**Antes:**
```go
	cajaComando.SetText("")
	if strings.EqualFold(comando, "bye") {
		time.Sleep(300 * time.Millisecond)
		ventana.Close()
	}
}
```

**Después:** se borran esas 4 líneas.
```go
	cajaComando.SetText("")
}
```

**Cómo explicarlo:** "El cliente envía `bye`; el servidor responde con un mensaje `bye` y cierra la sesión; la goroutine `recibirMensajes` recibe esa respuesta y cierra la ventana con `fyne.Do`. Hay un solo lugar que cierra la ventana, y el cierre ocurre cuando el servidor confirma." Si el servidor ya estaba caído, la ventana muestra "Conexion cerrada por el servidor." y se cierra con la X.

---

## 5. `Accept` sin pausa cuando falla (`servidor/main.go`, `main`)

**Problema:** si `Accept` falla (por ejemplo, cuando el proceso se queda sin descriptores de archivo porque hay demasiadas conexiones abiertas), el bucle hacía `continue` de inmediato. Volvía a fallar al instante, **en un ciclo que consumía el 100% de un núcleo** y llenaba la consola con cientos de miles de líneas por segundo.

**Antes:**
```go
if err != nil {
	fmt.Printf("Error aceptando conexion: %v\n", err)
	continue
}
```
**Después:**
```go
if err != nil {
	fmt.Printf("Error aceptando conexion: %v\n", err)
	time.Sleep(100 * time.Millisecond)
	continue
}
```

**Cómo explicarlo:** "Si aceptar una conexión falla, se espera 100 ms antes de reintentar, para no quedar en espera activa (*busy waiting*) consumiendo CPU."

---

## Pruebas realizadas

### Servidor
Se compilaron dos servidores: el de la rama `old` (antes) y el de `main` corregido (después). Se les corrió **el mismo script de pruebas** (un cliente en Python que habla el mismo protocolo JSON), cada prueba sobre una instancia nueva del servidor.

| Prueba | Antes | Después |
|---|---|---|
| CPU con los 4 núcleos al 100% | 17.8% | 100% |
| `ping 127.0.0.1` seguido de `bye` | Sin respuesta tras 15 s (sesión colgada) | Salida a los 10,0 s con el aviso; `bye` confirmado |
| Salida de 30 MB | Un mensaje de 29 297 KB | 64 KB + "[Salida recortada a 64 KB]" |
| `sleep 4 &` | Respuesta a los 4,0 s | Respuesta a los 1,0 s |
| Login con `periodo=9223372037` | 453 reportes en 1,5 s | 0 reportes (usa 5 s) |
| Descriptores agotados (límite forzado a 20) | 99% de CPU, 294 335 líneas de log/s | 0% de CPU, 10 líneas de log/s |
| 40 salidas y reportes enviados al mismo tiempo | Todos los mensajes JSON íntegros | Todos los mensajes JSON íntegros |
| **Uso normal** (clave mala, clave buena, `uname -s`, `echo hola; exit 3`, `ls /noexiste`, `true`, reporte, `bye`) | Correcto | **Idéntico a antes** |

Al terminar no quedó ningún proceso suelto (`ping`, `sleep` ni servidores).

### Cliente
Se usó el modo de pruebas de Fyne sin pantalla (`go test -tags ci`), que dibuja las ventanas por software. Se comparó el cliente antes de las correcciones con el corregido, ambos contra el servidor corregido.

| Prueba | Antes | Después |
|---|---|---|
| Periodo `abc` / `0` | Rechazado | Rechazado |
| Periodo `3600` | Aceptado | Aceptado |
| Periodo `3601` / `9223372037` | **Aceptado** | **Rechazado** con mensaje |
| Flujo Conexión → Login → OperGUI | Correcto (1 ventana abierta a la vez) | Correcto (1 ventana abierta a la vez) |
| `uname -s` en la GUI | `Linux` | `Linux` |
| Salida grande en la GUI | — | 64 KB + aviso de recorte |
| `ping 127.0.0.1` en la GUI | — | Aviso "Comando detenido…" a los 10 s |
| Reportes y barras | Llegan | Llegan |
| `bye` | GUI bloqueada 300 ms; la ventana se cerraba 2 veces | GUI bloqueada 0 ms; se cierra 1 vez y la app termina |

### Validación estática
- `gofmt`: sin archivos mal formateados.
- `go vet ./servidor` (Linux) y `go vet ./cliente` (Windows): sin advertencias.
- `sh compilar.sh`: genera `dist/linux-amd64/servidorOperativos` y `dist/windows-amd64/clienteOperativos.exe` (app gráfica de Windows).

---

## Limitaciones conocidas (no corregidas a propósito)

Se dejaron así para que el código siga siendo simple de explicar:

- **La contraseña viaja sin cifrar por TCP.** En producción se usaría TLS. El requisito pide validar con SHA-256 en el servidor.
- **SHA-256 sin sal:** lo pide el requisito; en un sistema real se usaría sal o bcrypt.
- **Los intentos fallidos se cuentan por conexión,** no por IP: reconectando se puede volver a intentar.
- **No hay comandos interactivos** (`top`, `vim`): no hay terminal, se ejecuta con `bash -c` y se captura la salida. Usar `top -bn1`.
- **Fin de la goroutine de reportes:** termina en su siguiente envío después de cerrar la sesión, porque escribir en el socket cerrado falla.
- **Escrituras de dos goroutines al mismo socket:** los mensajes no se mezclan porque cada mensaje es una sola llamada `Write` y Go serializa las escrituras sobre un mismo socket (verificado en las pruebas).
- **Comandos en segundo plano** (`sleep 100 &`) siguen corriendo hasta terminar, aunque el servidor ya no los espera.
