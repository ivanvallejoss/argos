# Argos

Panel de monitoreo fijo en casa (Lenovo Tab M7) con el estado de ivanvallejos.dev, el VPS, Bricka y la notebook.

```
tablet (Fennec)
  └─ https://hp-g4-vallejos.<tailnet>.ts.net  ── tailnet, puerto 443 ──┐
                                                                       ▼
hp-g4-vallejos                                            tailscale serve (HTTPS)
                                                                       │
                                                   argos 127.0.0.1:8081 (Go)
                                                                       │ cada 30 s
                                                      Gatus 127.0.0.1:8080 (SQLite)
                                                                       │ cada 60 s
                         ivanvallejos.dev · ping al VPS · Bricka · DNS de Google y Quad9
```

Gatus y argos escuchan solo dentro de la notebook. A la tablet le llega únicamente el JSON propio de argos
(estados ya resueltos, sin direcciones ni mensajes de error de Gatus) y la página.

## Archivos

| Ruta | Qué es |
|---|---|
| `gatus/config.yaml` | Chequeos. Va a `/etc/gatus/config.yaml`. |
| `deploy/gatus.service`, `deploy/argos.service` | Unidades de systemd con usuario propio y endurecimiento. |
| `deploy/gatus.env.ejemplo` | IP del VPS y dirección de Bricka (fuera del repo). Va a `/etc/gatus/gatus.env`. |
| `servicio/` | Argos en Go (solo biblioteca estándar), con la página embebida en `servicio/web/`. |

## Antes de empezar

- **Notebook por cable.** En wifi de 2,4 GHz el SSH se cortaba; los chequeos salen de ahí y darían caídas falsas.
- **Go en la Yoga.** Gatus v5.38.0 pide Go 1.26.3 o más nuevo para compilar. Con `GOTOOLCHAIN=auto` (el valor por
  defecto) Go baja solo la versión que haga falta.

## 1. Compilar en la Yoga

```sh
mkdir -p bin
CGO_ENABLED=0 GOBIN="$PWD/bin" go install github.com/TwiN/gatus/v5@v5.38.0
(cd servicio && go test ./... && CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o ../bin/argos .)

ssh hp-g4-vallejos mkdir -p /tmp/argos
scp bin/gatus bin/argos gatus/config.yaml deploy/* hp-g4-vallejos:/tmp/argos/
```

Los dos binarios son estáticos (Gatus usa SQLite en Go puro) y no necesitan AVX.

## 2. Regla de ping en el firewall de Hetzner

El chequeo del VPS es un ping a su IP pública, que hoy el firewall no deja pasar.

- IP de casa: `curl -4 -s https://ifconfig.me` desde la notebook (o la WAN en el MikroTik).
- Consola de Hetzner → Firewalls → el firewall del VPS → regla de entrada: protocolo **ICMP**, origen
  `<IP de casa>/32`, descripción "ping desde casa (argos)". Con la CLI:

  ```sh
  hcloud firewall add-rule <firewall> --direction in --protocol icmp \
    --source-ips <IP de casa>/32 --description "ping desde casa (argos)"
  ```

- Prueba desde la notebook: `ping -c 3 <IP del VPS>`. Si no responde, revisá ufw en el VPS (por defecto
  `/etc/ufw/before.rules` acepta el ping).

## 3. Instalar en la notebook

```sh
cd /tmp/argos
sudo install -m 0755 gatus argos /usr/local/bin/
sudo install -d -m 0755 /etc/gatus
sudo install -m 0644 config.yaml /etc/gatus/config.yaml
sudo install -m 0600 gatus.env.ejemplo /etc/gatus/gatus.env
sudoedit /etc/gatus/gatus.env          # VPS_IP y BRICKA_URL
sudo install -m 0644 gatus.service argos.service /etc/systemd/system/
sudo systemd-analyze verify /etc/systemd/system/gatus.service /etc/systemd/system/argos.service
```

El ping sin privilegios necesita que el grupo del servicio esté dentro de este rango (en Ubuntu suele venir
`0 2147483647`):

```sh
sysctl net.ipv4.ping_group_range
# Si muestra "1 0":
echo 'net.ipv4.ping_group_range = 0 2147483647' | sudo tee /etc/sysctl.d/60-ping.conf && sudo sysctl --system
```

Arrancar y verificar:

```sh
sudo systemctl daemon-reload
sudo systemctl enable --now gatus argos
systemctl status gatus argos --no-pager
sleep 90; curl -s http://127.0.0.1:8081/api/estado; echo
journalctl -u gatus -u argos -n 30 --no-pager
```

Para ver la página propia de Gatus mientras depurás: `ssh -L 8080:127.0.0.1:8080 hp-g4-vallejos` y abrir
`http://localhost:8080` en la Yoga.

## 4. Publicar en la tailnet

1. Consola de Tailscale → DNS: MagicDNS activo y **HTTPS Certificates → Enable**. Ahí te pide aceptar que los
   nombres de las máquinas y de la tailnet quedan en el registro público de certificados.
2. En la notebook:

   ```sh
   sudo tailscale serve --bg 8081
   tailscale serve status        # https://hp-g4-vallejos.<tailnet>.ts.net/ → proxy http://127.0.0.1:8081
   ```

   `--bg` deja la configuración guardada y sobrevive reinicios. El certificado se pide y se renueva solo.

## 5. Regla de Tailscale para la tablet

La única regla nueva. Si tu política usa `grants`:

```json
{ "src": ["tag:panel"], "dst": ["tag:homelab"], "ip": ["tcp:443"] }
```

Si usa `acls`:

```json
{ "action": "accept", "src": ["tag:panel"], "dst": ["tag:homelab:443"] }
```

Prueba para `tests` (junto a la que ya le niega SSH):

```json
{
  "src": "tag:panel",
  "accept": ["tag:homelab:443"],
  "deny": ["tag:homelab:22", "tag:homelab:8080", "tag:homelab:8081", "tag:server:22", "tag:server:443"]
}
```

## 6. En la tablet

1. En Fennec abrí `https://hp-g4-vallejos.<tailnet>.ts.net`: candado y panel cargado.
   Si el nombre no resuelve, lo primero a revisar es DNS sobre HTTPS en los ajustes de Fennec: en protección
   aumentada o máxima consulta un servidor externo que no conoce los nombres de Tailscale.
2. Menú de Fennec → instalar o agregar a la pantalla de inicio. Abierta desde el ícono ("Argos") no tiene barra de
   direcciones (las barras de Android quedan visibles; la página se adapta al alto que queda).
3. `about:support` → sección de gráficos, línea de composición: si dice "Software", el dibujado lo hace el
   procesador.
4. **Aislamiento por IP** (Termux no resuelve nombres de Tailscale). IP de la notebook: `tailscale ip -4` en ella.

   ```sh
   pkg install curl netcat-openbsd
   curl -sk -o /dev/null -w '%{http_code}\n' https://<IP Tailscale notebook>/   # 200
   nc -zvw3 <IP Tailscale notebook> 22                                          # tiene que fallar
   nc -zvw3 <IP Tailscale VPS> 22                                               # tiene que fallar
   ```

   `-k` solo para esta prueba: el certificado es para el nombre, no para la IP.
5. **Corte de internet.** Desconectá la WAN del MikroTik unos 3 minutos. En uno o dos minutos el panel tiene que
   pasar a "Se cortó internet en casa", y la tablet tiene que seguir llegando a la notebook por la red local.
6. **Batería y animaciones.** Mismo brillo, batería cerca del 100%, 20–30 minutos cada una, en una pestaña de
   Fennec (la app siempre abre `/`):
   - `https://hp-g4-vallejos.<tailnet>.ts.net/?medir=quieto` → sin animaciones ni transiciones.
   - `https://hp-g4-vallejos.<tailnet>.ts.net/?medir=animado` → el punto del pie late sin parar.

   Compará temperatura y corriente de la batería con la misma lectura de Termux que usaste para el cable. El tiempo
   de las animaciones constantes después de un toque está en `INACTIVIDAD_MS`, en `servicio/web/panel.js`.

## Cómo se lee el panel

Tarjetas (una por chequeo del grupo `sistemas`, en orden alfabético, más la notebook):

| Estado | Cuándo |
|---|---|
| Funciona | la última medición salió bien |
| Con fallas | falló la última, todavía no son 3 seguidas (por ejemplo: lento, o un corte de un minuto) |
| Caído | 3 fallas seguidas, unos 3 minutos |
| Sin dato | no hay medición de los últimos 3 minutos, o se cortó internet en casa |

Cartel general, en orden de prioridad:

1. **Sin conexión con la notebook**: la tablet falló 2 pedidos seguidos. Lo decide la página.
2. **La notebook no está midiendo**: Gatus no responde, o su última medición tiene más de 3 minutos.
3. **Se cortó internet en casa**: fallaron los dos controles de DNS. Los sistemas pasan a "Sin dato".
4. **Caído: …** / **Con fallas: …**
5. **Todo funciona**

Movimiento: transiciones de color de 0,3 s solo cuando cambia un estado, y tres parpadeos del texto al pasar a
"Caído", siempre activos. La única animación constante (el punto del pie) corre solo durante 2,5 minutos después
de un toque.

Si la app se recarga con la notebook caída, una copia guardada de la página (service worker, solo la página,
nunca los datos) muestra "Sin conexión con la notebook" en vez del error de Firefox.

## Mantenimiento

- **Agregar un sistema:** una entrada en `gatus/config.yaml` con `group: sistemas`, copiar a la notebook y
  `sudo systemctl restart gatus`. La tarjeta aparece sola.
- **Actualizar argos o la página:** compilar, copiar `argos` a `/usr/local/bin/` y `sudo systemctl restart argos`.
  La tablet detecta la versión nueva y se recarga sola.
- **Logs:** `journalctl -u gatus -u argos`. Gatus está en `WARN` para no escribir cada chequeo en el disco.

## Fuera de esta etapa

- **Avisos por WhatsApp:** en stand by hasta que bottero esté listo. Como corre en la misma notebook, Gatus le
  puede avisar con su proveedor genérico de webhooks sin tocar reglas de Tailscale.
- **Datos de cada sistema (Beszel):** segunda etapa, con su propia decisión sobre la regla entre el VPS y la notebook.
