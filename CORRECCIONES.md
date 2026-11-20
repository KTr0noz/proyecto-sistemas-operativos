# Correcciones de errores: antes vs después

Este documento compara el código **antes** y **después** de cada cambio, y resume las pruebas hechas al servidor y al cliente. Los cambios se hicieron en cinco rondas:
1. Los errores 1 a 5.
2. El `cd` persistente (error 6).
3. La ubicación actual en la GUI (7) y la lista de comandos no permitidos (8).
4. Los mensajes de error por IP o puerto equivocados en el cliente (9).
5. Los comandos que causaban errores: salidas sin fin, `pkill`, procesos en segundo plano, programas interactivos, colores, salidas grandes, `history`, etc. (10).

- **Antes:** la rama `old` (proyecto original) y el commit de la GUI de conexión/login, para el código del cliente.
- **Después:** la rama `main` con las correcciones.
- **Tamaño del cambio, ronda 1 (errores 1 a 5):** 2 archivos (`servidor/main.go` y `cliente/main.go`), +33 / −17 líneas, una sola función nueva (`leerTiemposCPU`) y un solo import nuevo (`context`). La estructura del programa no cambia.
- **Tamaño del cambio, ronda 2 (error 6):** `servidor/main.go` +28 / −2 líneas, ningún archivo ni función nueva y un solo import nuevo (`syscall`). Además, `compilar.ps1` pierde 6 líneas: el servidor de prueba para Windows (ver sección 6). El cliente no cambia.
- **Tamaño del cambio, ronda 3 (7 y 8):** `servidor/main.go` +34 / −2 y `cliente/main.go` +10 / −2. Hay una sola función nueva (`comandoPermitido`) y una lista (`comandosNoPermitidos`), y ningún import nuevo.
- **Tamaño del cambio, ronda 4 (9):** `cliente/main.go` +48 / −5, con una función nueva (`conectar`) y dos imports nuevos (`errors` y `syscall`), y `servidor/main.go` +6 / −0. La herramienta `medicion/main.go` también lee el saludo nuevo.
- **Tamaño del cambio, ronda 5 (10):** `servidor/main.go` +141 / −50 y `cliente/main.go` +29 / −9.
  - De las líneas nuevas del servidor, 27 son comentarios y unas 30 son el bloque que ejecuta bash, que pasó de `atenderCliente` a una función propia (`ejecutarComando`) casi sin cambios.
  - Hay un tipo nuevo (`salidaLimitada`), una función nueva (`revisarComando`, que reemplaza a `comandoPermitido`), una lista nueva (`comandosInteractivos`) y cuatro imports nuevos (`errors`, `path/filepath`, `regexp` y `slices`).
  - En el cliente cambia el widget del panel de salida (`TextGrid` en vez de `Label`) y se agregan `clear` y el corte de líneas muy largas.
  - El protocolo no cambia: `medirLatencia` funciona igual.

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
| 9 | IP o puerto equivocados sin un error claro | cliente + servidor | Con un puerto ocupado por otro programa, el login **se quedaba colgado sin mensaje**; los demás fallos mostraban el error técnico en inglés (`connection refused`, `i/o timeout`) | Mensaje claro en la ventana de conexión en los **10 casos probados**; nunca se llega al login con un puerto equivocado |
| 10 | Comandos que causaban errores | servidor + cliente | `yes` **tumbaba el servidor** (517 MB en 4 s); `pkill` lo mataba; `yes > /dev/null &` quedaba al **99,5% de CPU**; 64 KB de texto congelaban la GUI **8,2 s**; `vi`/`ssh` se colgaban **10 s**; colores y `nano` llenaban la ventana de símbolos; `history` vacío | `yes` responde en **0,02 s** y el servidor sigue en **12 MB**; `pkill` bloqueado; **ningún proceso** queda vivo; 64 KB se ven en **0,19 s**; mensajes claros e inmediatos; `history`, `cd -`, `clear`, `exit` y `logout` funcionan |

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

**Actualización (ronda 5):**
- El recorte a 64 KB ahora se hace **mientras** se lee la salida, porque `yes` alcanzaba a llenar la memoria antes de recortar (sección 10.1).
- Los procesos en segundo plano ya no quedan vivos y el error de `WaitDelay` se reemplazó por un aviso claro (sección 10.3).

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

**Actualización (ronda 5):** este bloque pasó a la función `ejecutarComando`. Además, `Setpgid` se cambió por `Setsid`, que también crea el grupo de procesos pero sin terminal (sección 10.4).

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

**Actualización (ronda 5):** `comandoPermitido` se reemplazó por `revisarComando`, que revisa el programa con el que empieza **cada parte** de la línea (`ls; rm x`, `/bin/rm x`). La lista pasó de 20 a 23 comandos: se agregaron `halt`, `pkill` y `killall` (sección 10.2).

---

## 9. IP o puerto equivocados sin un error claro (`cliente/main.go`, `conectar`; `servidor/main.go`, saludo)

**Problema:** el cliente solo sabía si la conexión TCP se abría o no.
- **Puerto equivocado pero con otro programa escuchando** (un servidor web, SSH, etc.): la conexión se abre igual, así que el cliente pasaba al login como si nada. Al presionar **Ingresar**:
  - si ese programa no responde, la ventana se quedaba **colgada para siempre**, sin mensaje y con el botón deshabilitado;
  - si responde otra cosa, salía "Conexion terminada por el servidor.", que no dice que el problema es el puerto.
- **IP o puerto mal escritos:** se mostraba el error técnico de Go, en inglés: `connect: connection refused`, `i/o timeout`, `lookup 192.168.1.300: no such host`, `unknown port`.

**Antes (cliente):**
```go
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
```

**Después (cliente):** se valida antes de conectar…
```go
if net.ParseIP(ip) == nil {
	textoEstado.SetText("Error: la IP \"" + ip + "\" no es valida. Escriba una IP como 192.168.1.10.")
	return
}
if numero, err := strconv.Atoi(puerto); err != nil || numero < 1 || numero > 65535 {
	textoEstado.SetText("Error: el puerto debe ser un numero entre 1 y 65535.")
	return
}
```
…y la conexión pasa a una función que devuelve el mensaje de error, si lo hay:
```go
func conectar(ip, puerto string) (net.Conn, *json.Decoder, string) {
	conexion, err := net.DialTimeout("tcp", net.JoinHostPort(ip, puerto), 5*time.Second)
	if err != nil {
		var codigo syscall.Errno
		// Conexion rechazada: la IP existe pero nadie escucha en ese puerto
		// (ECONNREFUSED en Linux, codigo 10061 en Windows).
		if errors.As(err, &codigo) && (codigo == syscall.ECONNREFUSED || codigo == 10061) {
			return nil, nil, fmt.Sprintf("Error: la IP %s respondio, pero el puerto %s esta cerrado o es equivocado (el servidor no esta escuchando en ese puerto).", ip, puerto)
		}
		return nil, nil, fmt.Sprintf("Error de conexion: la IP %s no responde. Verifique que la IP exista y que el servidor este encendido y en la misma red.", ip)
	}

	// El servidor saluda apenas acepta la conexion. Si en 5 s no llega el saludo,
	// en ese puerto hay otro programa que no es el servidor del proyecto.
	lector := json.NewDecoder(conexion)
	var saludo Mensaje
	conexion.SetReadDeadline(time.Now().Add(5 * time.Second))
	err = lector.Decode(&saludo)
	conexion.SetReadDeadline(time.Time{})

	if err != nil || (saludo.Tipo != "bienvenida" && saludo.Tipo != "rechazo") {
		conexion.Close()
		return nil, nil, fmt.Sprintf("Error: el puerto %s esta ocupado por otro programa o es equivocado (no respondio el servidor del proyecto).", puerto)
	}
	if saludo.Tipo == "rechazo" {
		conexion.Close()
		return nil, nil, "Error: " + saludo.Contenido
	}
	return conexion, lector, ""
}
```

**Después (servidor),** en `atenderCliente`:
```go
if config.IPPermitida != "*" && config.IPPermitida != ipCliente {
	fmt.Printf("[-] Conexion rechazada: IP %s no autorizada en servidor.conf\n", ipCliente)
	enviar(conexion, Mensaje{Tipo: "rechazo", Contenido: "la IP " + ipCliente + " no esta autorizada en el servidor."})
	return
}

// Saludo inicial: le confirma al cliente que en este puerto esta el servidor del proyecto.
if err := enviar(conexion, Mensaje{Tipo: "bienvenida", Contenido: "Servidor Operativos"}); err != nil {
	return
}
```

**Qué hace cada parte:**
- **`net.ParseIP` y el rango 1–65535:** atrapan los errores de escritura antes de intentar conectar, con un mensaje en español.
- **`syscall.Errno` 10061 / `ECONNREFUSED`:** es la respuesta del sistema cuando la IP existe pero nadie escucha en ese puerto. Ese error se traduce como "puerto cerrado o equivocado". Cualquier otro fallo (sin respuesta en 5 s, equipo inexistente, sin ruta) se traduce como "la IP no responde". Se comparan los dos códigos porque en Windows el error llega como 10061 y no como `ECONNREFUSED`.
- **El saludo (`bienvenida`):** el servidor lo envía apenas acepta la conexión. Si el cliente no lo recibe en 5 s (`SetReadDeadline`) o recibe algo que no es JSON del proyecto, en ese puerto hay otro programa. El error se muestra en la ventana de conexión, en vez de pasar al login.
- **`SetReadDeadline(time.Time{})`:** quita el límite de 5 s después del saludo, para que la sesión pueda durar lo que sea.
- **El `rechazo`:** cuando la IP del cliente no está en `ip_permitida`, el servidor ahora dice por qué cierra, en vez de cerrar en silencio.
- **El mismo `lector` pasa al login:** el saludo se leyó con ese decodificador, y crear otro perdería datos que ya estén en su búfer.

**Importante:** el cliente y el servidor nuevos van juntos. Un cliente viejo no espera el saludo y lo confunde con la respuesta del login. Un cliente nuevo con un servidor viejo no recibe el saludo y muestra "puerto ocupado o equivocado".

**Cómo explicarlo:** "El servidor saluda apenas alguien se conecta. Si el cliente no recibe ese saludo, sabe que en ese puerto no está nuestro servidor. Si ni siquiera puede conectarse, distingue si la IP no responde o si el puerto está cerrado, y lo dice en español."

---

## 10. Comandos que causaban errores (`servidor/main.go` y `cliente/main.go`)

En la ronda 4 se probaron comandos que podían dar problemas (`history`, `nano`, `vi`, `yes`, `pkill`, comandos en segundo plano, etc.) y solo se reportaron. En esta ronda se corrigieron todos, buscando en cada caso la forma más simple. Todos los "antes" y "después" están medidos (ver "Pruebas realizadas").

| Comando | Antes (medido) | Después (medido) | Ver |
|---|---|---|---|
| `yes`, `cat /dev/zero` (salida sin fin) | El servidor guardaba toda la salida en memoria: a los **4 s** llegó a **517 MB**, Linux lo mató y se cayeron **todas** las sesiones | Responde en **0,02 s** con 64 KB y aviso; el servidor sigue en **12 MB**, también con 6 sesiones a la vez | 10.1 |
| `pkill servidorOperati`, `killall servidorOperativos` | **Mataba los servidores** del mismo usuario | `comando no permitido` | 10.2 |
| `ls; rm x`, `/bin/rm x`, `\rm x`, `echo $(rm x)` | **Borraban el archivo** | `comando no permitido` | 10.2 |
| `sleep 300 &`, `yes > /dev/null &` | `Error ejecutando: exec: WaitDelay expired…` y los procesos **quedaban vivos** (`yes` al **99,5%** de CPU) | "[Los procesos en segundo plano (&) se detuvieron…]"; **no queda ningún proceso** | 10.3 |
| `vi archivo.txt` | **10 s** de espera, símbolos y un archivo `.archivo.txt.swp` | "comando interactivo no soportado: vi necesita una terminal." y una alternativa | 10.4 |
| `nano archivo.txt` | Símbolos de pantalla y un archivo de bloqueo `.swp` | Igual que `vi` | 10.4 |
| `top`, `watch date` | `top: failed tty get`; `watch`: **10 s** y símbolos | Mensaje con la alternativa (`top -bn1`) | 10.4 |
| `ssh operador@localhost`, `read -s < /dev/tty` | **10 s** detenidos esperando el teclado | Fallan **al instante** con el error del programa ("Host key verification failed.") | 10.4 |
| `ls --color=always`, `grep --color=always`, `printf '\033[31m…'`, `clear` | Símbolos como `[01;34m` en la ventana | Texto limpio; `clear` limpia el panel | 10.5 |
| `cat /usr/bin/ls` (binario) | 64 KB de símbolos; la ventana se congeló **10,1 s** | "[El comando devolvio datos binarios; no se muestran]" en **0,20 s** | 10.6 |
| `seq 1 15000` (64 KB de texto) | La ventana se congeló **8,2 s** | Se ve en **0,19 s**, con letra de terminal | 10.6 |
| `history` | Vacío | Lista numerada de los comandos de la sesión | 10.7 |
| `cd -` | `OLDPWD not set` | Vuelve a la carpeta anterior de la sesión | 10.7 |
| `exit`, `logout` | `exit` no hacía nada; `logout` daba error | Cierran la sesión, igual que `bye` | 10.7 |
| `export X=1` y luego `echo $X` | Vacío, sin explicación | Sigue vacío (cada comando es un bash nuevo), pero con un aviso de cómo hacerlo | 10.7 |
| Servidor iniciado como root | Arrancaba | No arranca: "por seguridad el servidor no se ejecuta como root" | 10.8 |
| Falta `servidor.conf` o el puerto está ocupado | Mensaje, pero código de salida **0** (como si todo estuviera bien) | Mensaje y código de salida **1** | 10.8 |

**Cambio de estructura:** el bloque que ejecuta bash salió de `atenderCliente` a una función propia, `ejecutarComando(comando, directorio, carpetaAnterior)`, que devuelve la salida lista y la carpeta donde quedó bash. Así `atenderCliente` queda corto: decide si el comando es `history`, si se rechaza o si se ejecuta, y envía la respuesta en un solo lugar.
```go
var salida string
if comando == "history" {
	// bash -c no guarda historial, asi que lo lleva el servidor.
	for i, c := range historial {
		salida += fmt.Sprintf("%5d  %s\n", i+1, c)
	}
} else if aviso := revisarComando(comando); aviso != "" {
	fmt.Printf("[-] Comando de '%s' no ejecutado: %s -> %s\n", usuario, comando, aviso)
	salida = aviso
} else {
	fmt.Printf("[*] Ejecutando comando de '%s' en %s: %s\n", usuario, directorio, comando)
	var nuevoDirectorio string
	salida, nuevoDirectorio = ejecutarComando(comando, directorio, carpetaAnterior)
	if nuevoDirectorio != directorio {
		carpetaAnterior, directorio = directorio, nuevoDirectorio
	}
}

if err := enviar(conexion, Mensaje{Tipo: "salida", Contenido: salida, Directorio: directorio}); err != nil {
	break
}
```

### 10.1 Salida sin fin: el límite de 64 KB ahora se aplica mientras se lee

**Problema:** `CombinedOutput()` guarda **toda** la salida en memoria, y el recorte a 64 KB se hacía **después**, cuando el comando ya había terminado. `yes` escribe cientos de MB por segundo y no termina nunca. El servidor llegó a 517 MB en 4 s y Linux lo mató por falta de memoria (*OOM killer*), cerrando las sesiones de todos los usuarios.

**Antes:**
```go
salida, err := comandoBash.CombinedOutput()
// ...
if len(salida) > 64*1024 {
	salida = append(salida[:64*1024], "\n[Salida recortada a 64 KB]\n"...)
}
```

**Después:**
```go
// salidaLimitada guarda la salida de un comando, pero como maximo 64 KB. Cuando se llena,
// Write devuelve un error: Go deja de leer y cierra la tuberia, y el comando que siga
// escribiendo (por ejemplo yes) termina solo. Asi la salida nunca ocupa mas de 64 KB.
type salidaLimitada struct {
	datos     []byte
	recortada bool
}

func (s *salidaLimitada) Write(p []byte) (int, error) {
	espacio := 64*1024 - len(s.datos)
	if len(p) > espacio {
		s.datos = append(s.datos, p[:espacio]...)
		s.recortada = true
		return espacio, errors.New("salida mayor a 64 KB")
	}
	s.datos = append(s.datos, p...)
	return len(p), nil
}
```
y en `ejecutarComando`:
```go
var salida salidaLimitada
bash.Stdout = &salida
bash.Stderr = &salida
err := bash.Run()
```

**Qué hace cada parte:**
- **`Write`:** es el método que Go llama cada vez que el comando escribe algo. Guarda hasta completar 64 KB.
- **Al llenarse, devuelve un error:** Go deja de leer y **cierra la tubería**. El comando que siga escribiendo (`yes`) recibe la señal `SIGPIPE` y termina solo, igual que pasa con `yes | head`.
- **`Stdout` y `Stderr` son el mismo objeto:** Go usa una sola tubería, así que los errores siguen mezclados con la salida, como con `CombinedOutput`.
- **`recortada`:** indica que hay que agregar el aviso "[Salida recortada a 64 KB]".
- **Si la salida se recortó,** no se busca la marca de la carpeta, porque bash no alcanzó a escribirla. Un `cd` hecho en ese mismo comando (`cd /tmp && yes`) no se mantiene.

### 10.2 `pkill`, `killall` y comandos encadenados (`revisarComando`)

**Problema:**
- `pkill` y `killall` no estaban en la lista. `pkill servidorOperati` mató al servidor y también a cualquier otro servidor del mismo usuario.
- La lista solo miraba la primera palabra. Por eso `ls; rm x`, `ls && /bin/rm x`, `\rm x` o `echo $(rm x)` sí se ejecutaban.

**Antes:** `comandoPermitido` (sección 8) comparaba solo `campos[0]` con la lista de 20 comandos.

**Después:** la lista tiene 23 comandos (se agregan `halt`, `pkill` y `killall`) y la revisión se hace en cada parte de la línea:
```go
var comandosNoPermitidos = []string{
	"su", "sudo", "passwd", "useradd", "userdel", "usermod",
	"shutdown", "reboot", "poweroff", "halt", "systemctl",
	"kill", "pkill", "killall",
	"rm", "chmod", "chown", "dd", "mkfs", "fdisk", "mount", "umount",
	"iptables",
}

func revisarComando(linea string) string {
	separador := func(r rune) bool { return strings.ContainsRune(";&|()`\n", r) }
	for _, parte := range strings.FieldsFunc(linea, separador) {
		palabras := strings.Fields(parte)
		if len(palabras) == 0 {
			continue
		}
		// Sin comillas ni ruta: "rm", \rm y /bin/rm cuentan como rm.
		programa := filepath.Base(strings.Trim(palabras[0], `"'\`))
		if slices.Contains(comandosNoPermitidos, programa) {
			return "comando no permitido"
		}

		consejo, interactivo := comandosInteractivos[programa]
		if programa == "top" && strings.Contains(parte, "-b") {
			interactivo = false // top -b (modo por lotes) si funciona sin terminal
		}
		if interactivo {
			return "comando interactivo no soportado: " + programa + " necesita una terminal.\n" + consejo
		}
	}
	return ""
}
```

**Qué hace cada parte:**
- **`strings.FieldsFunc(linea, separador)`:** corta la línea en partes en cada `;`, `&`, `|`, `(`, `)`, comilla invertida o salto de línea. Por ejemplo, `ls; rm x` da dos partes, `ls` y ` rm x`. Así se cubren `;`, `&&`, `||`, las tuberías, `$( )` y los subshells.
- **De cada parte se toma la primera palabra.** Se le quitan las comillas y la barra invertida (`"rm"`, `\rm`) y la ruta con `filepath.Base` (`/bin/rm` → `rm`).
- **`slices.Contains`:** busca esa palabra en la lista. Hace lo mismo que el `for` de antes, en una línea.
- **Devuelve un texto en vez de `true`/`false`:** el mensaje que se le envía al cliente, o `""` si el comando se puede ejecutar. Así la misma función sirve también para los programas interactivos (10.4).
- **Falsos positivos posibles:** `echo "a; rm"` se bloquea, porque la revisión es sobre el texto. No hace daño: solo no se ejecuta. `echo sudo`, `ls rm` y `rmdir vacia` se siguen permitiendo.
- **Sigue sin ser una barrera completa:** `bash -c "rm x"` o `x=rm; $x archivo` no se detectan. Por eso el servidor ya no arranca como root (10.8).

### 10.3 Procesos en segundo plano (`comando &`)

**Problema:** con `&`, bash termina enseguida pero el proceso sigue vivo con la tubería de salida abierta. El servidor esperaba 1 s (`WaitDelay`) y respondía con el error interno de Go `exec: WaitDelay expired before I/O complete`. El proceso **quedaba corriendo para siempre**: `yes > /dev/null &` se quedó al 99,5% de CPU. El grupo de procesos solo se mataba cuando se vencían los 10 s.

**Después,** en `ejecutarComando`:
```go
err := bash.Run()

// Si quedaron procesos en segundo plano (comando &), se matan con todo el grupo.
// Kill solo devuelve nil si el grupo todavia tenia procesos.
quedaronProcesos := bash.Process != nil && syscall.Kill(-bash.Process.Pid, syscall.SIGKILL) == nil
```
y en los avisos:
```go
case quedaronProcesos:
	texto += "\n[Los procesos en segundo plano (&) se detuvieron al terminar el comando]\n"
```

**Qué hace cada parte:**
- **Después de cada comando,** y no solo cuando se vence el tiempo, se manda `SIGKILL` al grupo completo, con el pid en negativo, igual que en la sección 6.
- **Si el comando terminó normal,** el grupo ya está vacío y `Kill` devuelve un error (`ESRCH`, "no existe ese proceso"), que se ignora.
- **Si devuelve `nil`,** había procesos en segundo plano: ya se mataron, y se avisa al usuario en vez de mostrar el error interno de Go.
- **`bash.Process != nil`:** evita un pánico si bash no llegó a arrancar.
- **Un comando con `&` sigue tardando 1 s** (el `WaitDelay`), pero ya no deja nada corriendo.

### 10.4 Programas que necesitan una terminal (`vi`, `nano`, `top`, `watch`, contraseñas)

**Problema:** el servidor se lanza desde una terminal de la VM y los comandos heredaban esa terminal, pero en otro grupo de procesos.
- Un programa que intentaba leer del teclado quedaba **detenido** por el sistema hasta los 10 s. Les pasaba a `vi`, a `ssh` (pregunta si confía en el equipo o pide la contraseña) y a `read < /dev/tty`.
- Los editores y `watch` mandaban además códigos de pantalla, y `nano` y `vi` dejaban archivos `.swp`.

**Antes:**
```go
comandoBash.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
```

**Después:**
```go
// TERM=dumb: los programas no envian colores ni codigos de pantalla.
// OLDPWD: la carpeta anterior de la sesion, para que funcione cd -.
bash.Env = append(os.Environ(), "TERM=dumb", "OLDPWD="+carpetaAnterior)
// Setsid: bash y sus hijos van en una sesion y un grupo de procesos propios, sin terminal.
// Asi se puede matar el grupo completo (pid negativo), y un programa que intente leer
// del teclado (una contrasena, por ejemplo) falla de inmediato en vez de quedarse esperando.
bash.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
```
y la lista de programas de pantalla completa, que revisa `revisarComando` (10.2):
```go
var comandosInteractivos = map[string]string{
	"nano":  "Use cat archivo para verlo o echo texto > archivo para escribirlo.",
	"vi":    "Use cat archivo para verlo o echo texto > archivo para escribirlo.",
	"vim":   "Use cat archivo para verlo o echo texto > archivo para escribirlo.",
	"top":   "Use top -bn1.",
	"htop":  "Use top -bn1.",
	"watch": "Ejecute el comando sin watch.",
}
```

**Qué hace cada parte:**
- **`Setsid`:** crea una **sesión** nueva **sin terminal**. También crea un grupo de procesos nuevo, por eso `Kill(-pid)` sigue funcionando igual. Un programa que abre `/dev/tty` recibe el error "No such device or address" y termina enseguida:
  - `ssh` responde "Host key verification failed.";
  - `read < /dev/tty` responde con ese error,

  en vez de quedarse 10 s detenidos.
- **`TERM=dumb`:** le dice a los programas que la "terminal" no soporta colores ni mover el cursor. Con eso, por ejemplo, `ls --color=always` deja de mandar colores.
- **`comandosInteractivos`:** son los editores (`nano`, `vi`, `vim`), `top`, `htop` y `watch`. No se ejecutan: responden "comando interactivo no soportado: X necesita una terminal." y una alternativa en la línea de abajo. Como no se ejecutan, no quedan archivos `.swp`.
- **`top -bn1`** (modo por lotes) sí se permite: es el que usa la medición del paper.
- **`less`, `more` y `man` no están en la lista,** porque sin terminal funcionan como `cat`.

### 10.5 Colores y `clear`

**Problema:** `ls --color=always`, `grep --color=always`, `printf '\033[31m…'` o `clear` mandan secuencias de escape (ESC `[` … letra) que la ventana no entiende y muestra como `[01;34m`.

**Después (servidor):**
```go
// Codigos de color y de pantalla de las terminales (ESC [ ... letra). La ventana del
// cliente no los entiende y los mostraria como simbolos raros.
var codigosTerminal = regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]`)
// ...
texto = codigosTerminal.ReplaceAllString(texto, "")
```

**Después (cliente),** en `enviarComando`:
```go
// clear: limpia el panel aqui mismo, no hace falta ir al servidor.
if comando == "clear" {
	textoSalida.SetText("")
	cajaComando.SetText("")
	return
}
```

**Qué hace cada parte:**
- **La expresión regular** busca el carácter ESC (`\x1b`), un `[`, números, `;` o `?`, y una letra final, que es la forma de todos los códigos de color. `ReplaceAllString` los borra y deja el texto.
- **`clear`:** lo que el usuario espera es limpiar el panel, así que el cliente lo hace sin ir al servidor.

### 10.6 Salidas grandes y binarias congelaban la ventana

**Problema:** el panel de salida era un `Label`. Fyne crea un objeto de texto por cada línea y los dibuja todos, aunque no se vean. Con 15 000 líneas (64 KB), la ventana se congeló 8,2 s. Un binario (`cat /usr/bin/ls`) además la llenaba de símbolos y la congeló 10,1 s.

**Antes (cliente):**
```go
textoSalida := widget.NewLabel("Esperando comando...")
panelSalida := container.NewScroll(textoSalida)
panelSalida.SetMinSize(fyne.NewSize(500, 180))
```

**Después (cliente):**
```go
// TextGrid: panel con letra monoespaciada, como una terminal. Solo dibuja las filas
// que se ven, por eso una salida grande no congela la ventana.
textoSalida := widget.NewTextGrid()
textoSalida.Scroll = fyne.ScrollBoth
textoSalida.SetText("Esperando comando...")
// ...
// La salida va en el centro y ocupa el espacio que sobra (crece con la ventana).
ventana.SetContent(container.NewBorder(arriba, abajo, nil, nil, textoSalida))
```
y en `recibirMensajes`, antes de mostrar la salida:
```go
// El panel reserva en cada fila tantas celdas como letras tenga la linea mas
// larga: las lineas muy largas se cortan para que la ventana no se ponga lenta.
lineas := strings.Split(salida, "\n")
for i, linea := range lineas {
	if letras := []rune(linea); len(letras) > 500 {
		lineas[i] = string(letras[:500]) + " ..."
	}
}
fyne.Do(func() {
	textoSalida.SetText(strings.Join(lineas, "\n"))
	textoSalida.ScrollToTop()
	// ...
})
```

**Después (servidor):**
```go
// Un archivo binario (por ejemplo cat /bin/ls) no es texto: llenaria la ventana de simbolos.
if strings.ContainsRune(texto, 0) {
	texto = "[El comando devolvio datos binarios; no se muestran]\n"
}
```

**Qué hace cada parte:**
- **`widget.NewTextGrid()`:** es el widget de Fyne pensado para terminales.
  - Usa letra monoespaciada, así que las columnas de `ls -la` o `ps aux` quedan alineadas.
  - Con `Scroll` activado, solo crea las filas que se ven en pantalla.
  - Tiene el mismo método `SetText` que el `Label`.
- **`container.NewBorder(arriba, abajo, nil, nil, textoSalida)`:** la ubicación y la caja de comandos van arriba, el reporte y las barras abajo, y el panel de salida ocupa el centro. Si se agranda la ventana, el panel crece.
- **El corte a 500 caracteres:** el `TextGrid` reserva, en cada fila visible, tantas celdas como letras tenga la línea más larga. Una sola línea de 64 KB, como `printf 'a%.0s' $(seq 1 70000)`, crearía decenas de miles de celdas por fila.
- **`ScrollToTop()`:** cada salida nueva se muestra desde el principio.
- **El binario:** un texto nunca tiene el byte 0, y los ejecutables, imágenes y comprimidos sí. Es la misma regla que usa `grep` para decir "Binary file matches".

### 10.7 `history`, `cd -`, `exit`, `logout` y `export`

**Problema:** cada comando se ejecuta en un `bash -c` nuevo, que no guarda historial ni recuerda la carpeta anterior.
- `history` salía vacío y `cd -` respondía "OLDPWD not set".
- `exit` terminaba ese bash sin cerrar la sesión, y `logout` daba error.
- `export X=1` no pasaba al siguiente comando, sin ninguna explicación.

**Después:**
```go
carpetaAnterior := ""  // para cd -
var historial []string // comandos de la sesion, para history
// ...
if strings.EqualFold(comando, "bye") || comando == "exit" || comando == "logout" {
	// ... se cierra la sesion igual que con bye
}
historial = append(historial, comando)
```
El `history` se responde en `atenderCliente` (ver "Cambio de estructura"), y en `ejecutarComando`:
```go
bash.Env = append(os.Environ(), "TERM=dumb", "OLDPWD="+carpetaAnterior)
// ...
// Cada comando es un bash nuevo: las variables y los alias no pasan al siguiente comando.
if strings.HasPrefix(comando, "export ") || strings.HasPrefix(comando, "alias ") {
	texto += "[Aviso: cada comando se ejecuta en un bash nuevo, asi que la variable\n o el alias no pasa al siguiente comando. Use: export X=1 && echo $X]\n"
}
```

**Qué hace cada parte:**
- **`historial`:** lista de la sesión, como `directorio`. Cada comando se agrega, y `history` la muestra numerada con el mismo formato de bash (`%5d  comando`).
- **`carpetaAnterior`:** cuando un comando cambia la carpeta, la vieja se guarda aquí y se le pasa a bash como `OLDPWD`, que es la variable que usa `cd -`. Al empezar la sesión está vacía, así que el primer `cd -` dice "OLDPWD not set", igual que en una terminal recién abierta.
- **`exit` y `logout`:** el servidor los trata igual que `bye`, y el cliente cierra la ventana al recibir la confirmación.
- **`export` y `alias`:** mantener variables entre comandos exigiría un bash vivo durante toda la sesión, lo que cambia el diseño del servidor. Se dejó el diseño simple y se avisa cómo hacerlo en un solo comando: `export X=1 && echo $X`.

### 10.8 Servidor como root y código de salida

**Problema:**
- Los comandos del cliente se ejecutan con el usuario del servidor. Como root, un comando que pase la lista negra (por ejemplo `bash -c "rm …"`) podría borrar cualquier cosa del sistema.
- Además, si el servidor no podía arrancar, terminaba con código 0, como si todo estuviera bien.

**Después,** al inicio de `main`:
```go
// Los comandos del cliente se ejecutan con el usuario del servidor: como root
// podrian borrar o cambiar cualquier cosa del sistema.
if os.Geteuid() == 0 {
	fmt.Println("Error: por seguridad el servidor no se ejecuta como root. Inicielo con un usuario normal.")
	os.Exit(1)
}
```
y en los errores de arranque, `os.Exit(1)` en vez de `return`.

**Qué hace cada parte:**
- **`os.Geteuid() == 0`:** el usuario efectivo 0 es root, también con `sudo`. El servidor no necesita root para nada: el puerto 2306 es mayor que 1024 y `/proc` se puede leer con cualquier usuario.
- **`os.Exit(1)`:** un código distinto de 0 avisa a la terminal o a un script que el servidor no arrancó.

**Cómo explicarlo:** "Cada comando corre en un bash nuevo, sin terminal y con límites. La salida se guarda solo hasta 64 KB, así un comando sin fin como `yes` termina solo. Al terminar se mata el grupo de procesos, así no queda nada en segundo plano. Los programas que necesitan una pantalla completa se rechazan con un mensaje, y la lista negra revisa cada parte de la línea. En el cliente, el panel es un `TextGrid`, que solo dibuja lo que se ve, y por eso las salidas grandes ya no congelan la ventana."

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

### Cliente (ronda 4: IP y puerto equivocados)
Se usó la GUI real del cliente, compilada para Linux y abierta en una pantalla virtual (Xvfb), manejada con mouse y teclado simulados (`xdotool`). Se comparó el cliente de antes (commit `df606cd`) con el corregido, cada uno contra su servidor. En los casos G y H, otro programa ocupaba el puerto: en G acepta la conexión y nunca responde, y en H responde con un saludo de SSH.

| Caso | Antes | Después |
|---|---|---|
| A. Puerto cerrado (`127.0.0.1:9999`) | `connect: connection refused` | "la IP 127.0.0.1 respondio, pero el puerto 9999 esta cerrado o es equivocado…" (inmediato) |
| B. IP sin equipo en la red | `connect: no route to host` | "la IP … no responde. Verifique que la IP exista…" (≈3 s) |
| C. IP que no contesta (`192.0.2.1`) | "Conectando…" y a los 5 s `i/o timeout` | "Conectando… (hasta 5 segundos)" y a los 5 s "la IP 192.0.2.1 no responde…" |
| D. IP mal escrita (`192.168.1.300`) | `lookup 192.168.1.300: no such host` | "la IP "192.168.1.300" no es valida…" (inmediato, sin conectar) |
| E. Puerto `abc` | `lookup tcp/abc: unknown port` | "el puerto debe ser un numero entre 1 y 65535." |
| F. Puerto `99999` | `address 99999: invalid port` | "el puerto debe ser un numero entre 1 y 65535." |
| G. Puerto ocupado por un programa mudo | pasaba al login y **se quedaba colgado** (sin mensaje después de 15 s) | a los 5 s: "el puerto 2399 esta ocupado por otro programa o es equivocado…"; no pasa al login |
| H. Puerto ocupado por otro servicio (SSH) | pasaba al login y salía "Conexion terminada por el servidor." | de inmediato: "el puerto 2398 esta ocupado por otro programa o es equivocado…" |
| I. IP no autorizada (`ip_permitida = 10.9.9.9`) | pasaba al login y salía "Conexion terminada por el servidor." | "la IP 127.0.0.1 no esta autorizada en el servidor."; el servidor lo registra |
| J. IP y puerto correctos | login → OperGUI | login → OperGUI, con la ubicación y los reportes, igual que antes |

**Uso normal con el servidor nuevo** (`medirLatencia`, que ya lee el saludo): login, los 25 comandos de prueba, el comando bloqueado, los reportes y `bye` funcionan. Contra el servidor con la IP restringida responde "la IP 127.0.0.1 no esta autorizada en el servidor.". En una comparación directa, alternando 300 conexiones contra cada servidor bajo la misma carga, el saludo agrega **0,26 ms** (mediana) a la conexión con autenticación.

### Servidor (ronda 5: comandos que causaban errores)
Se usó un contenedor Debian 13 (trixie) con 512 MB de memoria. Los dos servidores corrieron con el usuario normal `operador` y con terminal, como al lanzarlos desde una terminal de la VM:
- el de antes de esta ronda (ronda 4), en el puerto 2306;
- el corregido, en el 2307.

Se les envió el mismo script en Python que habla el protocolo JSON. Los resultados de cada comando están en la tabla de la sección 10. Además:

| Prueba | Antes | Después |
|---|---|---|
| 6 sesiones a la vez, cada una con `yes`, `cat /dev/zero`, `seq 1 9999999` y `yes` | — (con un solo `yes` el servidor muere) | Las 24 respuestas en 0,01–0,03 s; el servidor se mantuvo en 12 MB |
| `echo sudo`, `ls rm`, `rmdir vacia`, `top -bn1 \| head -3`, `man ls \| head -3`, `less archivo` | Se ejecutan | Se ejecutan igual (sin falsos positivos) |
| Uso normal: `uname -s`, `echo hola; exit 3`, `ls /noexiste`, `true`, `false`, `cd /usr && pwd`, `cd /noexiste`, `echo __DIRECTORIO__/falso`, `echo 'sin cerrar`, `ps aux \| head -2`, `ping -c 1 127.0.0.1` | — | Idéntico a antes |
| `sleep 12`, `ping 127.0.0.1` | Aviso a los 10,0 s | Aviso a los 10,0 s |
| `cd /tmp`, `cd /usr/share`, `cd -`, `cd -` | Error "OLDPWD not set" | `/tmp`, luego `/usr/share`; la ubicación se actualiza |
| `medirLatencia`, 3 corridas alternando los dos servidores | 14,90 ms de promedio general | 15,30 ms (las corridas variaron entre 14,50 y 16,22 ms por la carga del equipo) |
| Comparación directa: 500 ejecuciones alternadas de `pwd`, `uname -a`, `ls -la /etc` y `ps -e \| wc -l` | — | La mediana cambió entre **−0,12 y +0,07 ms**: las correcciones no cambian la latencia |
| Servidor como root | Arranca | "por seguridad el servidor no se ejecuta como root", código 1 |
| Sin `servidor.conf` / puerto ocupado | Mensaje, código 0 | Mensaje, código 1 |

Los comandos peligrosos de la lista (`shutdown`, `reboot`, `halt`, `mkfs`…) no se enviaron al servidor de antes. Al terminar no quedó ningún proceso suelto ni archivo `.swp`.

### Cliente (ronda 5: comandos que causaban errores)
Se usó la GUI real, compilada para Linux en una pantalla virtual (Xvfb) y manejada con `xdotool`. Se midió el tiempo desde que se presiona **Enter** hasta que la salida aparece en pantalla. El cliente de antes (con `Label`) y el nuevo (con `TextGrid`) se conectaron cada uno a su servidor. La pantalla virtual no tiene tarjeta gráfica, así que en Windows ambos deberían ser más rápidos; lo que importa es la diferencia.

| Comando | Cliente de antes | Cliente nuevo |
|---|---|---|
| `seq 1 15000` (64 KB, 15 000 líneas) | 8,19 s | 0,19 s |
| `seq 1 4000` (16 KB) | 1,04 s | 0,24 s |
| `cat /usr/bin/ls` | 10,11 s, símbolos | 0,20 s, aviso de datos binarios |
| Una sola línea de 64 KB (`printf 'a%.0s' $(seq 1 70000)`) | 1,61 s | 0,84 s, cortada a 500 caracteres |
| `ls -la /usr/bin` | 0,28 s, columnas desalineadas | 0,25 s, columnas alineadas |
| `ls --color=always -la /` | Símbolos `[01;34m` | Texto limpio |
| `nano archivo.txt` | Símbolos de pantalla | Mensaje en dos líneas con la alternativa |
| `clear` | Símbolos | Panel vacío |
| `exit` | No pasa nada | Se cierra la ventana y la app termina |
| Reportes cada 2 s mientras se ejecutan comandos | Llegan | Llegan |

Una vez mostrada la salida, una tecla escrita en la caja de comandos apareció en 0,05–0,34 s con el cliente de antes y en 0,05–0,20 s con el nuevo: en los dos, la ventana sigue respondiendo después de dibujar.

### Validación estática
Verificado otra vez después de la ronda 5:
- `gofmt`: sin archivos mal formateados.
- `go vet ./servidor` (Linux), `go vet ./cliente` (Windows) y `go vet ./medicion` (Linux y Windows): sin advertencias.
- `sh compilar.sh`: genera `dist/linux-amd64/servidorOperativos`, `dist/windows-amd64/clienteOperativos.exe` (app gráfica de Windows) y `dist/windows-amd64/medirLatencia.exe`.
- **Prueba rápida del servidor de `dist/`:** saludo, login, `pwd`, `nano x` (mensaje), `ls; pkill nada` (bloqueado), `history` y `exit` (cierra la sesión).

`compilar.ps1` no cambió en la ronda 5. En la ronda 4 se ejecutó con PowerShell 7, en una copia con las rutas adaptadas a Linux: terminó con "Compilacion exitosa" y generó el servidor Linux, el cliente `.exe` (app gráfica) y `medirLatencia.exe`.

---

## Limitaciones conocidas (no corregidas a propósito)

Se dejaron así para que el código siga siendo simple de explicar:

- **La contraseña viaja sin cifrar por TCP.** En producción se usaría TLS. El requisito pide validar con SHA-256 en el servidor.
- **SHA-256 sin sal:** lo pide el requisito; en un sistema real se usaría sal o bcrypt.
- **Los intentos fallidos se cuentan por conexión,** no por IP: reconectando se puede volver a intentar.
- **No hay comandos de pantalla completa** (`nano`, `vi`, `top` sin `-b`, `htop`, `watch`): no hay terminal, se ejecuta con `bash -c` y se captura la salida. El servidor responde con un mensaje y una alternativa (sección 10.4). Si se ejecuta otro programa de este tipo que no esté en la lista, no tiene terminal: falla enseguida o lo detiene el límite de 10 s.
- **Fin de la goroutine de reportes:** termina en su siguiente envío después de cerrar la sesión, porque escribir en el socket cerrado falla.
- **Escrituras de dos goroutines al mismo socket:** los mensajes no se mezclan porque cada mensaje es una sola llamada `Write` y Go serializa las escrituras sobre un mismo socket (verificado en las pruebas).
- **Un programa que se separa a propósito del grupo de procesos no se detiene** al terminar el comando. Por ejemplo, `setsid sleep 77 & sleep 0.3` deja `sleep 77` corriendo en su propia sesión (probado). Los procesos en segundo plano normales (`comando &`, `nohup comando &`) sí se detienen (sección 10.3).
- **Solo se mantienen la carpeta, la carpeta anterior (`cd -`) y el historial,** no las variables ni los alias: `export X=1` se pierde en el siguiente comando, porque cada comando es un bash nuevo. El servidor lo avisa y sugiere `export X=1 && comando`.
- **`history` solo funciona escrito solo:** `history | grep ls` o `history 5` van a bash, que no tiene historial, y salen vacíos.
- **La lista de comandos no permitidos revisa el texto de cada parte de la línea.** Bloquea `ls; rm x`, `/bin/rm x`, `\rm x` y `echo $(rm x)`, pero no:
  - variantes con otro nombre (`mkfs.ext4`),
  - comandos lanzados por otro programa (`bash -c "rm x"`, `find . -delete`),
  - ni comandos guardados en una variable (`x=rm; $x archivo`).

  Es un filtro de la interfaz, no una barrera de seguridad completa. La protección real es el usuario de Linux con el que corre el servidor; por eso el servidor no arranca como root.
- **La detección de binarios busca el byte 0.** Los ejecutables, imágenes y comprimidos siempre lo tienen. Unos pocos bytes al azar (`head -c 100 /dev/urandom`) pueden no tenerlo y se muestran como símbolos.
- **Las líneas de más de 500 caracteres se cortan en la ventana** (con " ..." al final). La salida completa, hasta 64 KB, sí llega al cliente.
- **Si la salida se recorta a 64 KB, se pierde el `cd` de ese mismo comando** (`cd /tmp && yes`), porque bash no alcanza a informar la carpeta.
- **Si se presiona Ctrl+C en la consola del servidor mientras corre un comando,** ese comando no recibe el Ctrl+C, porque está en su propia sesión: termina solo, al vencer los 10 s o cuando se cierra su salida.
