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
│   └── main.go            # Cliente Windows: login oculto en consola + ventana OperGUI (3 barras)
├── dist/                  # Binarios compilados listos para ejecutar
│   ├── linux-amd64/       # servidorOperativos (ELF estatico x86_64)
│   └── windows-amd64/     # clienteOperativos.exe (Windows GUI con Fyne)
├── compilar.ps1           # Script de compilacion en Windows (PowerShell)
├── compilar.sh            # Script de compilacion del servidor en Linux/WSL (sh)
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
```powershell
cd "c:\Carpetas\estudio\Universidad\Semestres\Cuarto Semestre\Operating Systems\Proyecto\dist\windows-amd64"
.\clienteOperativos.exe 127.0.0.1 2306 5
```

1. En la consola, ingresa usuario (`admin`) y contraseña (`linux`, sin eco en pantalla).
2. Al autenticarse correctamente, se abrirá la ventana **OperGUI (`Proyecto Operativos - Ramos`)**:
   * **Comandos UNIX:** Escribe comandos como `uname -a`, `ls -la`, `ps aux` en la caja de texto y presiona **Ejecutar** (o `Enter`). La salida aparece en el panel con desplazamiento sin mezclarse con los reportes.
   * **Monitoreo en paralelo:** Cada $n$ segundos se actualiza el texto de `Reporte #N` y las **3 barras de progreso** en tiempo real (**CPU**, **RAM** y **Disco**).
   * **Salir:** Escribe `bye` y presiona **Ejecutar** (o cierra la ventana).

---

## 4. Cómo Funciona el Código

### Servidor (`servidor/main.go`)
1. `leerConfiguracion("servidor.conf")`: Carga IP permitida, puerto, intentos fallidos y usuarios autorizados.
2. `autenticar(...)`: Valida usuario y compara el hash `sha256.Sum256` contra `usuarios.db` mediante `validarUsuario` y `usuarioPermitido`.
3. **Paralelismo:**
   * Goroutine periódica `enviarReportes`: lee `/proc/stat` (`leerCPU`), `/proc/meminfo` (`leerRAM`), procesos en `/proc` (`contarProcesos`) y `df /` (`leerDisco`) cada $n$ segundos, enviando el resumen y los porcentajes numéricos.
   * Bucle principal en `atenderCliente`: atiende comandos remotos ejecutando `bash -c <comando>` y cierra sesión al recibir `bye`.

### Cliente (`cliente/main.go`)
1. Valida los 3 argumentos de línea de comandos (`<IP> <Puerto> <PeriodoReporte>`).
2. `autenticar(...)`: Pide usuario y clave oculta (`term.ReadPassword`).
3. `iniciarGUI(...)`: Construye la ventana Fyne con caja de comandos, botón `Ejecutar`, panel de salida y 3 barras (`barraCPU`, `barraRAM`, `barraDisco`).
4. `recibirMensajes(...)`: Goroutine en segundo plano que actualiza la interfaz mediante `fyne.Do(...)` cada vez que llega un reporte o una salida de comando.
