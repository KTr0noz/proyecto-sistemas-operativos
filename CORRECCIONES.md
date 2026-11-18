# Correcciones de errores: antes vs después

Este documento compara el código **antes** y **después** de cada cambio, y resume las pruebas hechas al servidor y al cliente. Los cambios se hicieron en tres rondas:
1. Los errores 1 a 5.
2. El `cd` persistente (error 6).
3. La ubicación actual en la GUI (7) y la lista de comandos no permitidos (8).

- **Antes:** la rama `old` (proyecto original) y el commit de la GUI de conexión/login, para el código del cliente.
- **Después:** la rama `main` con las correcciones.
- **Tamaño del cambio, ronda 1 (errores 1 a 5):** 2 archivos (`servidor/main.go` y `cliente/main.go`), +33 / −17 líneas, una sola función nueva (`leerTiemposCPU`) y un solo import nuevo (`context`). La estructura del programa no cambia.
- **Tamaño del cambio, ronda 2 (error 6):** `servidor/main.go` +28 / −2 líneas, ningún archivo ni función nueva y un solo import nuevo (`syscall`). Además, `compilar.ps1` pierde 6 líneas: el servidor de prueba para Windows (ver sección 6). El cliente no cambia.
- **Tamaño del cambio, ronda 3 (7 y 8):** `servidor/main.go` +34 / −2 y `cliente/main.go` +10 / −2. Hay una sola función nueva (`comandoPermitido`) y una lista (`comandosNoPermitidos`), y ningún import nuevo.

## Resumen

| # | Cambio | Archivo | Antes (medido) | Después (medido) |
|---|---|---|---|---|
| 1 | CPU medida desde que arrancó el equipo | servidor | Con 4 núcleos al 100% reportaba **17.8%** | Reporta **100%** |
| 2 | Comandos sin límite de tiempo ni de tamaño | servidor | `ping` dejaba la sesión **colgada para siempre**; una salida de 30 MB se enviaba **completa** | `ping` se detiene a los **10 s** con aviso; la salida se recorta a **64 KB** |
| 3 | Periodo de reporte sin tope (desbordamiento) | servidor + cliente | `periodo=9223372037` producía **453 reportes en 1,5 s** | **0 reportes**; el cliente rechaza valores fuera de 1–3600 |
| 4 | Doble cierre de ventana con `bye` | cliente | La GUI se congelaba **300 ms** y la ventana se cerraba **2 veces** | **0 ms**; la ventana se cierra **1 vez**, cuando el servidor confirma |
| 5 | `Accept` sin pausa cuando falla | servidor | Con los descriptores agotados: **99% de CPU** y **294 335 líneas de log por segundo** | **0% de CPU** y **10 líneas por segundo** |
| 6 | `cd` no se mantenía entre comandos | servidor | `cd /usr` y luego `pwd` mostraba **la carpeta inicial**; `yes > /dev/null` quedaba **corriendo para siempre** tras el límite de 10 s | `pwd` muestra **`/usr`**; al vencer el tiempo **no queda ningún proceso** |
| 7 | Ubicación actual en la GUI | servidor + cliente | Solo se veía escribiendo `pwd` | Label **"Ubicacion actual: …"** desde el login, actualizado con cada comando |
| 8 | Lista de comandos no permitidos | servidor | `rm archivo` **borraba el archivo**; `su -` y `sudo` se ejecutaban | 20 comandos bloqueados: responde exactamente **`comando no permitido`** y no ejecuta nada |

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

## 6. `cd` no se mantenía entre comandos (`servidor/main.go`, `atenderCliente`)

**Problema:** cada comando se ejecuta en un proceso `bash` **nuevo**, hijo del servidor. `cd` es un comando interno de bash que cambia la carpeta **de ese proceso bash**; cuando bash termina, el cambio se pierde, y el siguiente comando arranca otra vez en la carpeta inicial. Un proceso **no puede cambiar la carpeta de su padre**. Por eso, en una terminal real, `cd` lo ejecuta el propio shell y no un programa aparte.

No sirve usar `os.Chdir` en el servidor, por dos razones:
- Cambiaría la carpeta de **todo el servidor**, es decir, de todas las sesiones a la vez.
- Rompería el login: `usuarios.db` se lee con ruta relativa.

**Antes:**
```go
fmt.Printf("[*] Ejecutando comando de '%s': %s\n", usuario, comando)
ctx, cancelar := context.WithTimeout(context.Background(), 10*time.Second)
comandoBash := exec.CommandContext(ctx, "bash", "-c", comando)
comandoBash.WaitDelay = time.Second
salida, err := comandoBash.CombinedOutput()
cancelar()
```

**Después:**
```go
// (antes del for) Carpeta donde se ejecutan los comandos de esta sesion.
carpetaInicial, _ := os.Getwd()
directorio := carpetaInicial

// (dentro del for)
// Si la carpeta guardada ya no existe (la borraron), se vuelve a la inicial.
if _, err := os.Stat(directorio); err != nil {
	directorio = carpetaInicial
}

fmt.Printf("[*] Ejecutando comando de '%s' en %s: %s\n", usuario, directorio, comando)
ctx, cancelar := context.WithTimeout(context.Background(), 10*time.Second)
// Al terminar, bash imprime la carpeta donde quedo (por si el comando hizo cd).
comandoBash := exec.CommandContext(ctx, "bash", "-c", `trap 'echo "__DIRECTORIO__$PWD"' EXIT; `+comando)
comandoBash.Dir = directorio
// bash y sus hijos van en un grupo de procesos propio: si se vence el tiempo
// se mata el grupo completo (pid negativo) y no quedan procesos huerfanos.
comandoBash.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
comandoBash.Cancel = func() error {
	return syscall.Kill(-comandoBash.Process.Pid, syscall.SIGKILL)
}
comandoBash.WaitDelay = time.Second
salida, err := comandoBash.CombinedOutput()
cancelar()

// Se separa la carpeta (lo que va despues de la marca) del resto de la salida.
texto := string(salida)
if i := strings.LastIndex(texto, "__DIRECTORIO__"); i >= 0 {
	directorio = strings.TrimSuffix(texto[i+len("__DIRECTORIO__"):], "\n")
	salida = salida[:i]
}
```

**Qué hace cada parte:**
- **`directorio`:** variable **propia de cada sesión**, porque se declara dentro de `atenderCliente` y cada cliente tiene su propia goroutine. Empieza en la carpeta del servidor, igual que antes.
- **`comandoBash.Dir = directorio`:** cada comando arranca en la carpeta guardada.
- **`trap '...' EXIT`:** le dice a bash "cuando termines, imprime `__DIRECTORIO__` seguido de tu carpeta actual (`$PWD`)".
  - Se ejecuta aunque el comando falle o haga `exit 3`.
  - No cambia el código de salida: `false` sigue mostrando `Error ejecutando: exit status 1`.
  - Va **antes** del comando del usuario. Así, si el comando está mal escrito (por ejemplo `echo 'sin cerrar`), bash rechaza la línea completa, no imprime la marca y la carpeta no cambia.
- **`strings.LastIndex(...)`:** busca la marca, guarda lo que va después como la nueva carpeta y la quita de la salida, así que el usuario nunca la ve.
  - Se usa la **última** aparición porque el `trap` es lo último que imprime bash: si el usuario escribe `echo __DIRECTORIO__/falso`, no engaña al servidor.
  - `TrimSuffix` quita el salto de línea que agrega `echo`.
- **`os.Stat(directorio)`:** si alguien borró la carpeta donde estaba la sesión, se vuelve a la carpeta inicial. Sin esto, Go no podría arrancar ningún comando en una carpeta inexistente y la sesión quedaría inutilizable.
- **`Setpgid` y `Cancel` (grupo de procesos):**
  - **Por qué hacen falta:** antes, con un comando simple como `sleep 60`, bash **se reemplazaba** por el comando (optimización de bash), así que matar "a bash" mataba al comando. Ahora bash tiene que seguir vivo para ejecutar el `trap`, así que el comando corre como su **hijo**. Si al vencer el tiempo solo se matara a bash, el hijo quedaría **huérfano** y seguiría corriendo.
  - **Qué hacen:** `Setpgid: true` crea un grupo de procesos nuevo para bash, y todos sus hijos lo heredan. `syscall.Kill(-pid, SIGKILL)`, con el pid en **negativo**, manda la señal al **grupo completo**.
  - **De paso se corrige un hueco que ya existía:** `yes > /dev/null` quedaba corriendo para siempre al 100% de un núcleo, porque bash no se reemplaza por comandos con redirección.
- **El log del servidor** ahora muestra en qué carpeta se ejecuta cada comando.

**Cómo se comporta:**

| Comando | Resultado |
|---|---|
| `cd /usr`, luego `pwd` | `/usr` |
| `cd bin` (relativo), `cd ..`, `cd` (solo, va a la carpeta personal), `cd "carpeta con espacios"` | Igual que en una terminal |
| `cd /etc && ls hostname` | Muestra `hostname` y la sesión queda en `/etc` |
| `cd /noexiste` | Mensaje de error de bash; la sesión **no se mueve** |
| `(cd /var)` | No se mantiene (subshell), igual que en una terminal |
| Dos clientes conectados | Cada uno tiene su propia carpeta |
| Se borra la carpeta donde está la sesión | El siguiente comando corre en la carpeta inicial |
| `cd /var; sleep 12` (se vence el tiempo) | El `cd` **no se mantiene**, porque el comando no terminó |

**Cambio en `compilar.ps1`:** se quitó el paso "Servidor de prueba para Windows", que generaba `dist/windows-amd64/servidorOperativos.exe` y una copia de `servidor.conf` y `usuarios.db` en esa carpeta.
- **Por qué:** `Setpgid` y `syscall.Kill` solo existen en Linux, así que ese paso ya no compilaba. Ese servidor de Windows, además, no podía medir CPU ni RAM (no hay `/proc`), necesitaba bash y no tenía las correcciones de la ronda 1.
- **Qué no cambia:** el cliente se compila en su propio paso y no depende del servidor ni de esos archivos.

**Cómo explicarlo:** "Cada comando corre en un proceso bash nuevo, y un proceso hijo no puede cambiar la carpeta de su padre; por eso el `cd` se perdía. Ahora el servidor guarda la carpeta de cada sesión, ejecuta cada comando en esa carpeta, y al terminar bash le avisa en qué carpeta quedó. Como bash se queda vivo hasta el final, el comando es su hijo; por eso, si se vence el tiempo, se mata al grupo de procesos completo y no solo a bash."

---

## 7. Ubicación actual en la GUI (`servidor/main.go` y `cliente/main.go`)

**Antes:** la carpeta de la sesión solo se podía ver escribiendo `pwd`.

**Después, en el protocolo:** `Mensaje` tiene un campo nuevo, igual en los dos programas.
```go
Directorio        string  `json:"directorio,omitempty"`
```

**Servidor:** manda la carpeta en el `auth_ok` (la carpeta inicial) y en cada `salida`.
```go
// autenticar
carpeta, _ := os.Getwd()
enviar(conexion, Mensaje{Tipo: "auth_ok", Contenido: "Autenticacion exitosa", Directorio: carpeta})

// atenderCliente
if err := enviar(conexion, Mensaje{Tipo: "salida", Contenido: string(salida), Directorio: directorio}); err != nil {
```

**Cliente:**
```go
// pedirLogin: la carpeta inicial llega con el auth_ok
iniciarGUI(programa, enviador, lector, respuesta.Directorio)

// iniciarGUI: el label va encima de la caja de comandos
textoUbicacion := widget.NewLabel("Ubicacion actual: " + directorio)
textoUbicacion.Wrapping = fyne.TextWrapBreak

// recibirMensajes, caso "salida": se actualiza junto con la salida del comando
textoSalida.SetText(salida)
textoUbicacion.SetText("Ubicacion actual: " + mensaje.Directorio)
```

**Qué hace cada parte:**
- **Solo con los comandos:** la carpeta viaja en cada `salida`, así que el label cambia solo cuando se ejecuta un comando y no con los reportes. Hay dos razones:
  - La carpeta solo puede cambiar con un comando.
  - `directorio` vive en la goroutine de comandos. Si los reportes también la enviaran, dos goroutines compartirían la variable y haría falta un mutex.
- **Desde el login:** el `auth_ok` trae la carpeta inicial, así el label ya se ve al abrir la ventana.
- **`TextWrapBreak`:** parte las rutas largas en varias líneas. Sin esto, una ruta larga obligaba a la ventana a medir 940 px de ancho (medido).

**Cómo explicarlo:** "El servidor ya sabía en qué carpeta está cada sesión. Ahora la manda junto con la salida de cada comando, y el cliente la muestra en un label con `fyne.Do`, igual que los reportes."

---

## 8. Lista de comandos no permitidos (`servidor/main.go`, `comandoPermitido`)

**Antes:** el servidor ejecutaba cualquier comando. Por ejemplo, `rm` borraba archivos y `su -` o `sudo` intentaban cambiar de usuario.

**Después:**
```go
// Comandos administrativos que no se permiten ejecutar desde el cliente.
var comandosNoPermitidos = []string{
	"su", "sudo", "passwd", "useradd", "userdel", "usermod",
	"shutdown", "reboot", "poweroff", "systemctl", "kill",
	"rm", "chmod", "chown", "dd", "mkfs", "fdisk", "mount", "umount",
	"iptables",
}

// Revisa solo el comando base (la primera palabra), sin importar sus argumentos.
func comandoPermitido(comando string) bool {
	campos := strings.Fields(comando)
	if len(campos) == 0 {
		return true
	}
	for _, noPermitido := range comandosNoPermitidos {
		if campos[0] == noPermitido {
			return false
		}
	}
	return true
}
```
En `atenderCliente`, justo después de revisar el `bye` y antes de ejecutar:
```go
if !comandoPermitido(comando) {
	fmt.Printf("[-] Comando no permitido de '%s': %s\n", usuario, comando)
	if err := enviar(conexion, Mensaje{Tipo: "salida", Contenido: "comando no permitido", Directorio: directorio}); err != nil {
		break
	}
	continue
}
```

**Qué hace cada parte:**
- **`strings.Fields(comando)`:** separa el comando en palabras (por espacios o tabs), y `campos[0]` es el comando base. Por eso `su`, `su -` y `  sudo   ls` se bloquean igual.
- **`len(campos) == 0`:** si el comando viene vacío, no se intenta leer `campos[0]`, que daría un pánico (*index out of range*).
- **El `for`:** recorre la lista con el mismo estilo que `usuarioPermitido`. La comparación es exacta, así que `rmdir`, `echo sudo` o `ls rm` sí se permiten.
- **Si está en la lista:** el servidor responde exactamente `comando no permitido` y anota el intento en su consola. Con `continue` vuelve a esperar el siguiente comando sin ejecutar nada.

**Cómo explicarlo:** "Antes de ejecutar, el servidor toma la primera palabra del comando y la busca en la lista. Si está, no se ejecuta y se responde 'comando no permitido'."

---

## Pruebas realizadas

### Servidor (ronda 1: errores 1 a 5)
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

### Cliente (ronda 1: errores 1 a 5)
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

### Servidor (ronda 2: `cd` persistente)
Mismo método: el servidor de antes de esta corrección (commit `ddd2126`) contra el corregido, con el mismo script en Python. "Inicial" es la carpeta desde donde se lanzó el servidor.

| Prueba | Antes | Después |
|---|---|---|
| `cd /usr` → `pwd` | inicial | `/usr` |
| `cd bin` → `pwd` | inicial | `/usr/bin` |
| `cd ..` → `pwd` | inicial | `/usr` |
| `cd /noexiste` → `pwd` | error de bash → inicial | mismo error → `/usr` (no se mueve) |
| `cd /etc && ls hostname` → `pwd` | `hostname` → inicial | `hostname` → `/etc` |
| `(cd /var)` → `pwd` | inicial | `/etc` (sin cambio, como en una terminal) |
| `cd` → `pwd` | inicial | carpeta personal |
| `cd "carpeta con espacios"` → `ls` | archivos del servidor | archivos de esa carpeta |
| Segundo cliente conectado al mismo tiempo | login correcto; `pwd` = inicial | login correcto; `pwd` = inicial, y el primero sigue en su carpeta |
| Se borra la carpeta donde está la sesión | — | vuelve a la inicial y el siguiente comando funciona |
| `echo __DIRECTORIO__/falso` → `pwd` | se muestra tal cual | se muestra tal cual; la carpeta no cambia |
| `cd /var; sleep 12` | aviso a los 10,0 s | aviso a los 10,0 s; la carpeta no cambia |
| `yes > /dev/null` (se vence el tiempo) | **queda huérfano al 100% de CPU** | aviso a los 10,0 s, **sin procesos huérfanos** |
| `sleep 25` y `ping` (se vence el tiempo) | aviso a los 10,0 s, sin huérfanos | aviso a los 10,0 s, sin huérfanos |
| `sleep 4 &` | respuesta a los 1,0 s | respuesta a los 1,0 s |
| `false`, `echo hola; exit 3`, `ls /noexiste`, `true`, `printf hola`, `echo 'sin cerrar`, salida de 100 KB, `uname -s`, reportes, `bye` | — | **idénticos a antes** |

Durante las pruebas se detectó que, sin el grupo de procesos, `sleep 25` quedaba huérfano después del límite (el aviso llegaba a los 11 s). Por eso se agregaron `Setpgid` y `Cancel`.

Al terminar no quedó ningún proceso suelto (`ping`, `yes`, `sleep` ni servidores).

### Cliente (ronda 2: `cd` persistente)
El código del cliente no cambió. Se probó la GUI real sin pantalla (`go test -tags ci`): ventana de conexión → login → OperGUI, escribiendo los comandos en la caja y presionando **Ejecutar**, contra los dos servidores.

| Comando en la GUI | Con el servidor de antes | Con el servidor corregido |
|---|---|---|
| `pwd` | inicial | inicial |
| `cd /usr` → `pwd` | `(comando ejecutado sin salida)` → inicial | `(comando ejecutado sin salida)` → `/usr` |
| `cd share` → `pwd` | error "No existe" → inicial | sin salida → `/usr/share` |
| `ls -d ../bin` | error "No existe" | `../bin` |
| `cd /noexiste` → `pwd` | error → inicial | error → `/usr/share` |
| `uname -s` | `Linux` | `Linux` |
| `bye` | se cierra la ventana (0 abiertas) | se cierra la ventana (0 abiertas) |

### Servidor (ronda 3: ubicación y comandos no permitidos)
- **La función `comandoPermitido`, probada sola** (sin ejecutar nada):
  - Se probaron los 20 comandos con 4 variantes cada uno: solo, con `-`, con argumentos (`-rf /tmp/x`), y con espacios y tab alrededor. Se bloquearon **80 de 80**.
  - Se permiten `ls -la`, `rmdir vacia`, `echo sudo`, `ls rm`, `cat /etc/passwd`, `ps aux`, `cd /usr` y el comando vacío.
- **De punta a punta,** el servidor de antes (commit `0211941`) contra el corregido. Solo se enviaron comandos de la lista que no hacen daño, sobre archivos de prueba, porque el servidor de antes sí los ejecuta. Los comandos peligrosos de la lista (`shutdown`, `reboot`, `mkfs`…) solo se probaron con la función.

| Prueba | Antes | Después |
|---|---|---|
| `auth_ok` | sin carpeta | trae la carpeta del servidor |
| `cd /usr`, `cd /noexiste` | sin carpeta | `/usr` y `/usr` (el `cd` fallido no mueve) |
| `rm archivo.txt`, `rm -rf carpeta` | **los borraba** | `comando no permitido`; siguen existiendo |
| `chmod 000 archivo.txt` | se ejecutaba | `comando no permitido`; los permisos siguen en 644 |
| `su -` | pedía contraseña y fallaba | `comando no permitido` |
| `sudo -n true`, `  sudo   -n true`, `kill -0 1` | se ejecutaban | `comando no permitido` |
| `rmdir vacia`, `echo sudo`, `ls -d /usr/bin` | se ejecutan | se ejecutan igual |
| `cd /var; sleep 12` | aviso a los 10,0 s | aviso a los 10,1 s; la carpeta sigue en `/usr` |
| `bye` | correcto | correcto |

### Cliente (ronda 3: ubicación y comandos no permitidos)
GUI real sin pantalla (`go test -tags ci`), contra el servidor corregido:

| Paso en la GUI | Label "Ubicacion actual" | Salida |
|---|---|---|
| Al abrir la ventana tras el login | carpeta del servidor | — |
| `cd /usr` | `/usr` | `(comando ejecutado sin salida)` |
| `sudo ls`, `su -`, `rm -rf /tmp/nada` | `/usr` | `comando no permitido` |
| `cd /noexiste` | `/usr` | error de bash |
| `cd share`, luego `pwd` | `/usr/share` | `/usr/share` |
| Llega un reporte | sin cambios | — |
| `cd` a una ruta de 150 caracteres | se parte en varias líneas; el label pide 16 px de ancho mínimo (940 px sin `Wrapping`) | — |
| `bye` | — | se cierra la ventana (0 abiertas) |

### Validación estática
- `gofmt`: sin archivos mal formateados.
- `go vet ./servidor` (Linux) y `go vet ./cliente` (Windows): sin advertencias.
- `sh compilar.sh`: genera `dist/linux-amd64/servidorOperativos` y `dist/windows-amd64/clienteOperativos.exe` (app gráfica de Windows).
- `compilar.ps1` se ejecutó con PowerShell 7 (en una copia con las rutas adaptadas a Linux). Termina con "Compilacion exitosa" y genera el servidor Linux y el cliente `.exe`, que es una app gráfica.

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
- **Solo se mantiene la carpeta,** no las variables: `export X=1` se pierde en el siguiente comando, porque cada comando es un bash nuevo.
- **`cd -` no vuelve a la carpeta anterior de la sesión,** porque cada bash nuevo no la conoce. Según cómo se lanzó el servidor, responde "OLDPWD no establecido" o va a la carpeta anterior de la terminal donde se arrancó el servidor.
- **La lista de comandos no permitidos revisa solo la primera palabra.** No bloquea:
  - rutas completas (`/bin/rm`),
  - comandos encadenados (`ls; rm x`),
  - variantes con otro nombre (`mkfs.ext4`),
  - ni comandos lanzados por otro programa (`bash -c "rm x"`).

  Es un filtro de la interfaz, no una barrera de seguridad completa. La protección real es el usuario de Linux con el que corre el servidor.
- **Si se presiona Ctrl+C en la consola del servidor mientras corre un comando,** ese comando no recibe el Ctrl+C, porque está en su propio grupo de procesos: termina solo o cuando se cierra su salida.
