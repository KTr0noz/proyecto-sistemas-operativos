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
├── dist/                  # Binarios compilados listos para ejecutar
│   ├── linux-amd64/       # servidorOperativos (ELF estatico x86_64)
│   └── windows-amd64/     # clienteOperativos.exe (Windows GUI con Fyne)
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

### Paso 2: Conectar el Cliente desde Windows
Abre `dist\windows-amd64\clienteOperativos.exe` con doble clic (es una app grafica, no abre consola). Tambien se puede lanzar con los parametros para que la primera ventana aparezca ya llena:
```powershell
cd "c:\Carpetas\estudio\Universidad\Semestres\Cuarto Semestre\Operating Systems\Proyecto\dist\windows-amd64"
.\clienteOperativos.exe 127.0.0.1 2306 5
```

1. **Ventana de conexion (`Proyecto Operativos - Conexion`):** escribe la IP del servidor, el puerto (`2306`) y el periodo de reporte en segundos (de 1 a 3600), y presiona **Conectar** (o `Enter`).
2. **Ventana de login (`Proyecto Operativos - Login`):** al conectar, la ventana anterior se cierra. Ingresa usuario (`admin`) y contraseña (`linux`, oculta) y presiona **Ingresar** (o `Enter`).
3. Al autenticarse correctamente, se abrirá la ventana **OperGUI (`Proyecto Operativos - Ramos`)**:
   * **Comandos UNIX:** Escribe comandos como `uname -a`, `ls -la`, `ps aux` en la caja de texto y presiona **Ejecutar** (o `Enter`). La salida aparece en el panel con desplazamiento sin mezclarse con los reportes. Un `cd` (por ejemplo `cd /tmp`) se mantiene para los comandos siguientes.
   * **Ubicacion actual:** encima de la caja de comandos se muestra la carpeta del servidor donde se ejecutan los comandos; se actualiza con cada comando.
   * **Comandos no permitidos:** los comandos administrativos de la lista (`su`, `sudo`, `rm`, `shutdown`, `systemctl`, etc.) no se ejecutan y responden `comando no permitido`.
   * **Monitoreo en paralelo:** Cada $n$ segundos se actualiza el texto de `Reporte #N` y las **3 barras de progreso** en tiempo real (**CPU**, **RAM** y **Disco**).
   * **Salir:** Escribe `bye` y presiona **Ejecutar** (o cierra la ventana).

---

## 4. Cómo Funciona el Código

### Servidor (`servidor/main.go`)
1. `leerConfiguracion("servidor.conf")`: Carga IP permitida, puerto, intentos fallidos y usuarios autorizados.
2. `autenticar(...)`: Valida usuario y compara el hash `sha256.Sum256` contra `usuarios.db` mediante `validarUsuario` y `usuarioPermitido`.
3. **Paralelismo:**
   * Goroutine periódica `enviarReportes`: lee `/proc/stat` (`leerCPU`, con dos muestras separadas 0,5 s), `/proc/meminfo` (`leerRAM`), procesos en `/proc` (`contarProcesos`) y `df /` (`leerDisco`) cada $n$ segundos, enviando el resumen y los porcentajes numéricos.
   * Bucle principal en `atenderCliente`: atiende comandos remotos ejecutando `bash -c <comando>` (máximo 10 segundos y 64 KB de salida) y cierra sesión al recibir `bye`.
   * Cada sesión guarda su carpeta actual en la variable `directorio`. Cada comando se ejecuta ahí y, al terminar, bash informa en qué carpeta quedó (`trap ... EXIT`), así un `cd` se mantiene entre comandos. Si se vence el tiempo, se mata el grupo de procesos completo (`Setpgid`), para no dejar procesos huérfanos.
   * Antes de ejecutar, `comandoPermitido` revisa la primera palabra del comando contra `comandosNoPermitidos` (20 comandos administrativos). Si está en la lista, responde `comando no permitido` sin ejecutar nada.
   * La carpeta de la sesión viaja en el campo `Directorio` del `auth_ok` y de cada `salida`.

### Cliente (`cliente/main.go`)
1. `pedirParametros(...)`: Ventana de conexion con IP, puerto y periodo (se llenan solos si se pasan `<IP> <Puerto> <PeriodoReporte>` al ejecutar). Al presionar **Conectar** valida el periodo y abre el socket con `net.DialTimeout` dentro de una goroutine, para que la ventana no se congele.
2. `pedirLogin(...)`: Ventana de login con usuario y contraseña (`widget.NewPasswordEntry`). `autenticar(...)` envia las credenciales y espera la respuesta del servidor, tambien en una goroutine.
   * Cada goroutine vuelve al hilo de la GUI con `fyne.Do(...)` para cambiar de ventana: primero abre la nueva y luego cierra la anterior, porque Fyne termina la app cuando no queda ninguna ventana abierta.
3. `iniciarGUI(...)`: Construye la ventana Fyne con la ubicación actual (`textoUbicacion`), caja de comandos, botón `Ejecutar`, panel de salida y 3 barras (`barraCPU`, `barraRAM`, `barraDisco`).
4. `recibirMensajes(...)`: Goroutine en segundo plano que actualiza la interfaz mediante `fyne.Do(...)` cada vez que llega un reporte o una salida de comando. La ubicación se actualiza solo con las salidas de comandos, no con los reportes.
