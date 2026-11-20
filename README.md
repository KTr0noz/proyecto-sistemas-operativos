# Proyecto Final de Sistemas Operativos

Plataforma cliente-servidor multiplataforma (**GNU/Linux** y **Windows**) para la **monitorización periódica de recursos** y la **ejecución remota de comandos UNIX** mediante sockets TCP, goroutines e interfaz gráfica **Fyne (OperGUI)** en **Go**.

---

## 1. Estructura del Proyecto

```text
Proyecto/
├── servidor/
│   ├── main.go            # Servidor TCP Linux: lee .conf, valida SHA-256, mide /proc y ejecuta bash
│   ├── servidor.conf      # Archivo plano de configuracion (IP, puerto, intentos, usuarios permitidos)
│   └── usuarios.db        # Base de datos plana (usuario:hash_sha256)
├── cliente/
│   └── main.go            # Cliente Windows (app grafica): ventana de conexion -> login -> OperGUI (3 barras)
├── medicion/
│   └── main.go            # Herramienta de medicion de latencia usada en el paper (seccion V)
├── dist/                  # Binarios compilados listos para ejecutar
│   ├── linux-amd64/       # servidorOperativos (ELF estatico x86_64)
│   └── windows-amd64/     # clienteOperativos.exe (Windows GUI con Fyne) y medirLatencia.exe
├── Paper_Final_Sistemas_Operativos_IEEE_v2.docx  # Paper IEEE actualizado (Windows 11 + VM Debian 13 en puente)
├── CORRECCIONES.md        # Errores corregidos: codigo antes vs despues y pruebas
├── compilar.ps1           # Script de compilacion en Windows (PowerShell)
├── compilar.sh            # Script de compilacion en Linux: servidor + cliente Windows (sh, requiere mingw-w64)
└── README.md
```

---

## 2. Usuarios y Claves de Prueba

El archivo `usuarios.db` contiene las credenciales con hash SHA-256:

| Usuario | Contraseña | Hash SHA-256 |
| :--- | :--- | :--- |
| `admin` | `linux` | `caf90169eefa5f807d577486b9f795ab86ae2983c5c20806cff959117e90af18` |
| `juan` | `1234` | `03ac674216f3e15c761ee1a5e255f067953623c8b388b4459e13f978d7c846f4` |
| `operador` | `operativos` | `6321fec14750d8eeb1abe35b1a8fc3b3c555863db75053b4d607559b2ef8cecd` |

---

## 3. Guía de Ejecución (WSL Ubuntu + Windows PowerShell)

### Paso 1: Iniciar el Servidor en Linux / WSL
```bash
cd "/mnt/c/Carpetas/estudio/Universidad/Semestres/Cuarto Semestre/Operating Systems/Proyecto/dist/linux-amd64"
chmod +x servidorOperativos
./servidorOperativos
```
El servidor se inicia con un usuario normal (sin `sudo`). Si se intenta como root, muestra "por seguridad el servidor no se ejecuta como root" y no arranca, porque los comandos del cliente se ejecutan con el usuario del servidor.

### Paso 2: Conectar el Cliente desde Windows
Abre `dist\windows-amd64\clienteOperativos.exe` con doble clic (es una app grafica, no abre consola). Tambien se puede lanzar con los parametros para que la primera ventana aparezca ya llena:
```powershell
cd "c:\Carpetas\estudio\Universidad\Semestres\Cuarto Semestre\Operating Systems\Proyecto\dist\windows-amd64"
.\clienteOperativos.exe 127.0.0.1 2306 5
```

1. **Ventana de conexion (`Proyecto Operativos - Conexion`):** escribe la IP del servidor, el puerto (`2306`) y el periodo de reporte en segundos (de 1 a 3600), y presiona **Conectar** (o `Enter`). Si algo falla, el error aparece en esa misma ventana: IP mal escrita, puerto fuera de 1–65535, "la IP no responde", "el puerto esta cerrado o es equivocado", "el puerto esta ocupado por otro programa" o "la IP no esta autorizada en el servidor".
2. **Ventana de login (`Proyecto Operativos - Login`):** al conectar, la ventana anterior se cierra. Ingresa usuario (`admin`) y contraseña (`linux`, oculta) y presiona **Ingresar** (o `Enter`).
3. Al autenticarse correctamente, se abrirá la ventana **OperGUI (`Proyecto Operativos - Ramos`)**:
   * **Comandos UNIX:** Escribe comandos como `uname -a`, `ls -la`, `ps aux` en la caja de texto y presiona **Ejecutar** (o `Enter`). La salida aparece en el panel central, con letra de terminal y desplazamiento, sin mezclarse con los reportes. Un `cd` (por ejemplo `cd /tmp`) se mantiene para los comandos siguientes, y `cd -` vuelve a la carpeta anterior.
   * **Ubicacion actual:** encima de la caja de comandos se muestra la carpeta del servidor donde se ejecutan los comandos; se actualiza con cada comando.
   * **Comandos no permitidos:** los 23 comandos administrativos de la lista (`su`, `sudo`, `rm`, `kill`, `pkill`, `killall`, `shutdown`, `systemctl`, etc.) no se ejecutan y responden `comando no permitido`. Se revisa cada parte de la línea, así que `ls; rm x` o `/bin/rm x` también se bloquean.
   * **Comandos de pantalla completa:** `nano`, `vi`, `vim`, `top` (sin `-b`), `htop` y `watch` necesitan una terminal real; responden "comando interactivo no soportado" con una alternativa (por ejemplo `top -bn1`).
   * **Límites de cada comando:** 10 segundos y 64 KB de salida. Los procesos en segundo plano (`comando &`) se detienen al terminar el comando; las salidas binarias (`cat /bin/ls`) se reemplazan por un aviso y los colores se quitan.
   * **Otros:** `history` muestra los comandos de la sesión y `clear` limpia el panel.
   * **Monitoreo en paralelo:** Cada $n$ segundos se actualiza el texto de `Reporte #N` y las **3 barras de progreso** en tiempo real (**CPU**, **RAM** y **Disco**).
   * **Salir:** Escribe `bye`, `exit` o `logout` y presiona **Ejecutar** (o cierra la ventana).

---

## 4. Cómo Funciona el Código

### Servidor (`servidor/main.go`)
1. `leerConfiguracion("servidor.conf")`: Carga IP permitida, puerto, intentos fallidos y usuarios autorizados.
2. `autenticar(...)`: Valida usuario y compara el hash `sha256.Sum256` contra `usuarios.db` mediante `validarUsuario` y `usuarioPermitido`.
   * Antes de autenticar, al aceptar la conexión, el servidor envía un saludo (`bienvenida`). Si la IP del cliente no coincide con `ip_permitida`, en vez del saludo envía un `rechazo` con el motivo y cierra la conexión.
3. **Paralelismo:**
   * Goroutine periódica `enviarReportes`: lee `/proc/stat` (`leerCPU`, con dos muestras separadas 0,5 s), `/proc/meminfo` (`leerRAM`), procesos en `/proc` (`contarProcesos`) y `df /` (`leerDisco`) cada $n$ segundos, enviando el resumen y los porcentajes numéricos.
   * Bucle principal en `atenderCliente`: recibe los comandos, responde `history` con el historial de la sesión, rechaza los comandos que no se pueden ejecutar y cierra sesión al recibir `bye`, `exit` o `logout`.
   * `ejecutarComando(...)` ejecuta `bash -c <comando>` con un máximo de 10 segundos y 64 KB de salida:
     * La salida se guarda en `salidaLimitada`, que deja de leer al llegar a 64 KB. Así un comando sin fin como `yes` termina solo y la memoria del servidor no crece.
     * bash corre en una sesión propia sin terminal (`Setsid`) y con `TERM=dumb`. Un programa que pide el teclado falla al instante en vez de quedarse esperando.
     * Al terminar cada comando se mata el grupo de procesos completo, así no quedan procesos en segundo plano ni huérfanos.
     * Se quitan los códigos de color (`codigosTerminal`) y una salida binaria se reemplaza por un aviso.
   * Cada sesión guarda su carpeta actual en la variable `directorio` y la anterior en `carpetaAnterior` (para `cd -`). Cada comando se ejecuta ahí y, al terminar, bash informa en qué carpeta quedó (`trap ... EXIT`), así un `cd` se mantiene entre comandos.
   * Antes de ejecutar, `revisarComando` separa la línea en partes (por `;`, `&`, `|`, paréntesis y comillas invertidas) y revisa el programa con el que empieza cada una:
     * Si está en `comandosNoPermitidos` (23 comandos administrativos), responde `comando no permitido`.
     * Si está en `comandosInteractivos` (`nano`, `vi`, `top`…), responde con un mensaje y una alternativa.
   * La carpeta de la sesión viaja en el campo `Directorio` del `auth_ok` y de cada `salida`.
   * El servidor no arranca como root, y si no puede arrancar termina con código 1.

### Cliente (`cliente/main.go`)
1. `pedirParametros(...)`: Ventana de conexion con IP, puerto y periodo (se llenan solos si se pasan `<IP> <Puerto> <PeriodoReporte>` al ejecutar). Al presionar **Conectar** valida el periodo, la IP y el puerto, y llama a `conectar(...)` dentro de una goroutine, para que la ventana no se congele.
   * `conectar(...)` abre el socket con `net.DialTimeout` (5 s) y espera el saludo del servidor (otros 5 s como máximo). Si falla, devuelve un mensaje claro: la IP no responde, el puerto está cerrado, el puerto está ocupado por otro programa o la IP no está autorizada.
2. `pedirLogin(...)`: Ventana de login con usuario y contraseña (`widget.NewPasswordEntry`). `autenticar(...)` envia las credenciales y espera la respuesta del servidor, tambien en una goroutine.
   * Cada goroutine vuelve al hilo de la GUI con `fyne.Do(...)` para cambiar de ventana: primero abre la nueva y luego cierra la anterior, porque Fyne termina la app cuando no queda ninguna ventana abierta.
3. `iniciarGUI(...)`: Construye la ventana Fyne con la ubicación actual (`textoUbicacion`), caja de comandos, botón `Ejecutar`, panel de salida y 3 barras (`barraCPU`, `barraRAM`, `barraDisco`).
   * El panel de salida es un `widget.TextGrid` (letra monoespaciada, como una terminal) en el centro de un `container.NewBorder`, así crece con la ventana. Solo dibuja las filas visibles, por eso una salida de 64 KB se muestra sin congelar la ventana.
   * `clear` limpia el panel en el cliente, sin ir al servidor.
4. `recibirMensajes(...)`: Goroutine en segundo plano que actualiza la interfaz mediante `fyne.Do(...)` cada vez que llega un reporte o una salida de comando. La ubicación se actualiza solo con las salidas de comandos, no con los reportes. Las líneas de más de 500 caracteres se cortan antes de mostrarlas.

---

## 5. Medir la Latencia (Paper)

`medirLatencia` se conecta y autentica igual que el cliente y mide cuánto tarda cada comando en volver (envío → salida). Prueba 25 comandos en 5 categorías (simples, con parámetros, con tuberías, dobles con `&&` y consultas de CPU en uso), 20 veces cada uno. También mide la conexión, la autenticación, un comando bloqueado, el retardo del reporte periódico y la memoria del servidor.

Con el servidor ya corriendo, desde PowerShell en Windows:
```powershell
cd dist\windows-amd64
.\medirLatencia.exe <IP-del-servidor> 2306 admin linux 20
```
Tarda alrededor de un minuto. Al final imprime el promedio por categoría y el promedio general, que son los valores de las Tablas VI y VII del paper.
