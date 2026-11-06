# Proyecto Final de Sistemas Operativos

Plataforma cliente-servidor multiplataforma (**GNU/Linux** y **Windows**) para la **monitorización periódica de recursos** y la **ejecución remota de comandos UNIX** mediante sockets TCP y concurrencia con goroutines en **Go**.

---

## 1. Estructura Simplificada del Proyecto

```text
Proyecto/
├── servidor/
│   ├── main.go            # Servidor TCP Linux: lee .conf, valida SHA-256, goroutine de /proc y ejecuta bash
│   ├── servidor.conf      # Archivo plano de configuracion (IP, puerto, intentos, usuarios permitidos)
│   └── usuarios.db        # Base de datos plana de usuarios y hashes SHA-256
├── cliente/
│   └── main.go            # Cliente TCP Windows: valida CLI, captura clave sin eco y terminal concurrente
├── dist/                  # Binarios compilados listos para ejecutar
│   ├── linux-amd64/       # servidorOperativos (ELF estatico x86_64) + clienteOperativos
│   └── windows-amd64/     # clienteOperativos.exe (Windows x86_64)
├── compilar.ps1           # Script de compilacion en Windows (PowerShell)
├── compilar.sh            # Script de compilacion en Linux/WSL (sh)
├── Paper_Final_Sistemas_Operativos_IEEE.docx
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

## 3. Guía de Ejecución

### Opción A: Probar en tu PC (WSL Ubuntu + Windows PowerShell)

#### Paso 1: Iniciar el Servidor en Linux / WSL
Abre una terminal de **Ubuntu (WSL)** y ejecuta:

```bash
cd "c:/Carpetas/estudio/Universidad/Semestres/Cuarto Semestre/Operating Systems/Proyecto/dist/linux-amd64"
chmod +x servidorOperativos
./servidorOperativos
```

#### Paso 2: Conectar el Cliente desde Windows
Abre **PowerShell** y ejecuta los 3 parámetros pedidos por el profesor (`<IP> <Puerto> <PeriodoReporte>`):

```powershell
cd "c:\Carpetas\estudio\Universidad\Semestres\Cuarto Semestre\Operating Systems\Proyecto\dist\windows-amd64"
.\clienteOperativos.exe 127.0.0.1 2306 5
```

1. Ingresa el usuario: `admin`
2. Ingresa la contraseña: `linux` (no se muestra en pantalla mientras escribes)
3. ¡Listo! Ya estás conectado:
   - Cada 5 segundos verás el reporte de recursos de Linux (`CPU`, `RAM`, `Procesos`, `Disco`).
   - Puedes escribir cualquier comando UNIX: `uname -a`, `ls -la`, `pwd`, `uptime`, `whoami`.
   - Para salir, escribe: `bye`.

---

## 4. Cómo Explicar el Código al Profesor (Resumen de 2 Minutos)

### Servidor (`servidor/main.go`):
1. **Configuración:** `cargarConfig("servidor.conf")` lee los parámetros planos de red e intentos permitidos.
2. **Socket TCP:** `net.Listen("tcp", ":"+cfg.Puerto)` abre el socket y escucha con `Accept()`.
3. **Autenticación SHA-256:** `validarCredenciales()` lee `usuarios.db` y compara el hash `sha256.Sum256([]byte(password))`.
4. **Concurrencia (Goroutines en paralelo):**
   - **Goroutine 1 (`ticker`):** Cada $n$ segundos lee `/proc/stat` (CPU), `/proc/meminfo` (RAM), cuenta carpetas en `/proc` (procesos) y consulta `df -h /` (disco), enviando el reporte por el socket.
   - **Goroutine 2 (Bucle principal):** Recibe comandos remotos, los ejecuta con `exec.Command("bash", "-c", cmd)` y devuelve la salida. Si recibe `bye`, cierra la sesión.

### Cliente (`cliente/main.go`):
1. **Parámetros CLI:** Valida que `os.Args` reciba `IP`, `Puerto` y `Periodo`.
2. **Conexión:** Se conecta con `net.Dial("tcp", ip+":"+puerto)`.
3. **Seguridad en Terminal:** Lee la contraseña sin eco con `term.ReadPassword(int(os.Stdin.Fd()))`.
4. **Concurrencia:** Una goroutine asíncrona escucha y presenta las métricas que llegan del servidor mientras el hilo principal lee lo que el usuario tipea por teclado.
