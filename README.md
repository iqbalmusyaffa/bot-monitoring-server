# 🖥️ Host Monitoring Telegram Bot (Golang)

Personal lightweight, secure, and production-ready Host Monitoring Telegram Bot built with Go and SQLite. Designed specifically for low-resource environments (Ubuntu Linux VPS with 1 vCPU and 1 GB RAM).

---

## 📌 1. Project Overview
Bot Telegram mandiri untuk memantau status ketersediaan (*availability*), *latency*, respons protokol HTTP/HTTPS, dan port TCP pada **domain, public IPv4, dan public IPv6**. Bot berjalan secara otomatis di latar belakang dan mengirimkan notifikasi instan ke Telegram hanya ketika terjadi perubahan status (misal: `ONLINE -> DOWN` atau `DOWN -> RECOVERED`).

---

## 🚀 2. Features
* **Zero Bloat & Lightweight**: 100% menggunakan Go Standard Library & SQLite (CGO-free). Memori runtime < 25 MB RAM.
* **Smart Target Detection**: Otomatis mendeteksi tipe target (`DOMAIN`, `IPV4`, `IPV6`).
* **Multi-Protocol Probing**:
  - **Domain**: DNS resolution (A & AAAA records), HTTP (Port 80), HTTPS (Port 443 dengan validasi sertifikat TLS), Latency.
  - **IP (v4 & v6)**: TCP Probe port (*non-root* ICMP fallback & RST packet reachability), TCP Port checks (default 22, 80, 443).
* **Robust Security & SSRF Protection**: Menolak seluruh rentang IP privat/internal, loopback, link-local metadata cloud (`169.254.169.254`), dan proteksi *DNS rebinding*.
* **Status-Change Alerting (No Spam)**: Hanya mengirim notifikasi saat status berubah (`ONLINE -> DOWN` atau `DOWN -> ONLINE`).
* **Resource Boundary Guard**: Batas goroutine concurrent, timeout ketat per koneksi jaringan, pembatasan body response, dan batasan redirect.
* **Telegram User Whitelist & Rate Limiter**: Akses terbatas hanya untuk Telegram User ID yang terdaftar dengan pembatasan frekuensi perintah (*sliding-window rate limit*).
* **Automatic History & Retention**: Menyimpan riwayat pemeriksaan ke SQLite dengan pembersihan log otomatis berkala (*retention policy*).

---

## 🎯 3. Supported Target
| Tipe | Contoh Input | Pemeriksaan |
| :--- | :--- | :--- |
| **Domain** | `example.com`, `google.com`, `api.domain.id` | DNS Resolution, HTTP, HTTPS (TLS Check), Latency |
| **IPv4** | `123.123.123.123`, `1.1.1.1` | Connectivity Probe, TCP Port (22, 80, 443), Latency |
| **IPv6** | `2001:db8::1`, `2606:4700:4700::1111` | Connectivity Probe, TCP Port (22, 80, 443), Latency |

*(Alamat IP private/internal seperti `127.0.0.1`, `10.x.x.x`, `192.168.x.x`, `::1` akan ditolak demi keamanan)*.

---

## 🏗️ 4. Architecture

```text
               +-------------------------------------------------------+
               |                  Telegram Bot API                     |
               +---------------------------+---------------------------+
                                           | Long Polling (Outbound)
                                           v
+--------------------------------------------------------------------------------------+
|                                  HOST MONITOR APP                                    |
|                                                                                      |
|  +--------------------+     +---------------------+     +-------------------------+  |
|  |   Authenticator    | --> |  Sliding-Window     | --> |    Command Dispatcher   |  |
|  | (User ID Whitelist)|     |    Rate Limiter     |     | (/start, /check, /add..) |  |
|  +--------------------+     +---------------------+     +------------+------------+  |
|                                                                      |               |
|                                                                      v               |
|  +--------------------------------------------------------------------------------+  |
|  |                           Security & SSRF Validator                            |  |
|  |         - Format Sanitization               - Private/Reserved IP Blocking     |  |
|  |         - DNS Rebinding Filter              - Payload Size Truncation          |  |
|  +-----------------------------------+--------------------------------------------+  |
|                                      |                                               |
|                                      v                                               |
|  +--------------------------------------------------------------------------------+  |
|  |                     Diagnostic Checker Engine (Pure Go)                        |  |
|  |   +------------------+  +-------------------+  +----------------------------+  |  |
|  |   |   DNS Checker    |  | HTTP/HTTPS Checker|  |   TCP & Connectivity Probe |  |  |
|  |   +------------------+  +-------------------+  +----------------------------+  |  |
|  +-----------------------------------+--------------------------------------------+  |
|                                      |                                               |
|                                      v                                               |
|  +------------------------+                     +---------------------------------+  |
|  |   Periodic Scheduler   | ------------------> |      SQLite Database (WAL)      |  |
|  | (Worker Pool Queue: 5) | <------------------ | hosts, checks history & cleanup |  |
|  +-----------+------------+                     +---------------------------------+  |
|              |                                                                       |
|              | (Status Changed: ONLINE <-> DOWN)                                     |
|              v                                                                       |
|  +------------------------+                                                          |
|  |  Notification Manager  | -----------------------------------> [Telegram Alert]    |
|  +------------------------+                                                          |
+--------------------------------------------------------------------------------------+
```

---

## 🛡️ 5. Security Architecture
1. **SSRF & DNS Rebinding Filter**:
   - Memblokir `127.0.0.0/8`, `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`, `169.254.0.0/16`, `0.0.0.0/8`, `224.0.0.0/4`, `240.0.0.0/4`, `::1`, `fc00::/7`, `fe80::/10`, `ff00::/8`.
   - Domain di-resolve ke IP terlebih dahulu; jika *salah satu* resolved IP teridentifikasi sebagai IP private, request langsung digagalkan.
2. **Command Injection Prevention**:
   - 0% pemanggilan shell (`sh -c`, `exec.Command`). Tidak menggunakan CLI eksternal (`curl`, `ping`, `nc`).
3. **Telegram Zero-Trust & Privacy**:
   - Akses hanya diberikan kepada user ID yang ada di whitelist.
   - User tidak sah hanya menerima pesan `❌ Unauthorized.` tanpa bocoran info internal.
4. **Non-Root & Systemd Hardening**:
   - Dijalankan di bawah user service `hostmonitor` dengan `NoNewPrivileges=true`, `ProtectSystem=strict`, `ProtectHome=true`.

---

## 📦 6. Installation
Pastikan Golang stable (1.22+) telah terpasang:
```bash
git clone https://github.com/your-username/host-monitor.git
cd host-monitor
go mod download
```

---

## 🤖 7. Telegram Bot Setup
1. **Dapatkan Token Bot**:
   - Buka Telegram, cari [@BotFather](https://t.me/BotFather).
   - Kirim `/newbot`, ikuti petunjuk nama bot, dan simpan HTTP API Token.
2. **Otomatisasi Admin (Auto-Claim First User)**:
   - Anda **tidak wajib** mengisi `ALLOWED_TELEGRAM_USER_IDS` di awal!
   - Saat bot pertama kali dijalankan, **orang pertama yang mengetik `/start` akan otomatis terdeteksi dan diangkat menjadi Owner / Admin Utama** ke database SQLite.
   - Bot langsung membuka akses menu penuh untuk Anda secara otomatis.
   - Selanjutnya, Anda dapat mendaftarkan teman/admin lain langsung dari Telegram menggunakan perintah: `/adduser <telegram_user_id>`.
3. (Opsional) Jika ingin mengatur whitelist manual via `.env`:
   ```env
   TELEGRAM_BOT_TOKEN=123456789:ABCdefGhIJKlmNoPQRsTUVwxyZ
   ALLOWED_TELEGRAM_USER_IDS=123456789
   ```

---

## ⚙️ 8. Environment Variables

| Variabel | Default | Keterangan |
| :--- | :--- | :--- |
| `TELEGRAM_BOT_TOKEN` | *(Wajib)* | Token Telegram Bot dari [@BotFather](https://t.me/BotFather) |
| `ALLOWED_TELEGRAM_USER_IDS` | *(Opsional)* | Whitelist ID Telegram manual (atau biarkan kosong untuk Auto-Claim) |
| `CHECK_INTERVAL` | `1m` | Interval monitoring otomatis |
| `REQUEST_TIMEOUT` | `10s` | Batas waktu timeout koneksi jaringan |
| `MAX_HOSTS` | `50` | Batas maksimum host yang dapat didaftarkan |
| `MAX_CONCURRENT_CHECKS` | `5` | Jumlah worker simultan pengecekan host |
| `MAX_RESPONSE_BODY` | `1048576` | Batas pembacaan body response (1 MB) |
| `MAX_REDIRECTS` | `5` | Batas pengalihan redirect HTTP |
| `MONITOR_PORTS` | `22,80,443` | Port whitelist untuk TCP probe |
| `COMMAND_RATE_LIMIT` | `10` | Batas jumlah perintah Telegram per window |
| `COMMAND_RATE_WINDOW` | `1m` | Durasi window rate limiter |
| `DB_PATH` | `data/monitor.db` | Lokasi file database SQLite |
| `RETENTION_DAYS` | `7` | Durasi penyimpanan histori pengecekan |
| `LOG_LEVEL` | `INFO` | Level log (`DEBUG`, `INFO`, `WARN`, `ERROR`) |

---

## 💬 9. Commands & Roles

### 👑 Daftar Perintah Berdasarkan Role:

| Perintah | Contoh | Hak Akses | Deskripsi |
| :--- | :--- | :--- | :--- |
| `/start` | `/start` | Semua Role | Menampilkan menu dan bantuan |
| `/help` | `/help` | Semua Role | Menampilkan panduan lengkap |
| `/check <host>` | `/check example.com` | Semua Role | Mengecek kondisi host secara instan |
| `/list` | `/list` | Semua Role | Menampilkan daftar seluruh host yang dimonitor |
| `/status <host>` | `/status example.com` | Semua Role | Melihat status terakhir & riwayat di database |
| `/add <host>` | `/add 123.123.123.123` | **Admin / Owner** | Menambahkan host ke monitoring otomatis |
| `/remove <host>` | `/remove example.com` | **Admin / Owner** | Menghapus host dari daftar monitoring |
| `/monitor <host>` | `/monitor example.com` | **Admin / Owner** | Mengaktifkan kembali monitoring host |
| `/unmonitor <host>`| `/unmonitor example.com` | **Admin / Owner** | Menjeda monitoring otomatis untuk host |
| `/listusers` | `/listusers` | **Owner Only** | Melihat seluruh daftar user dan rolenya |
| `/adduser <id> [role]` | `/adduser 987654321 user` | **Owner Only** | Mendaftarkan user baru dengan role `admin` / `user` |
| `/removeuser <id>` | `/removeuser 987654321` | **Owner Only** | Mencabut akses user |

---

## 💻 10. Local Development
1. Salin konfigurasi:
   ```bash
   cp .env.example .env
   ```
2. Jalankan aplikasi langsung dari source code:
   ```bash
   go run ./cmd/bot/main.go
   ```
3. Kompilasi binary mandiri:
   ```bash
   go build -ldflags="-s -w" -o host-monitor ./cmd/bot
   ```

---

## 🧪 11. Testing
Jalankan seluruh test suite unit & integration:
```bash
go test -v ./...
```
Untuk menguji package spesifik:
```bash
go test -v ./internal/security/...
go test -v ./internal/monitor/...
go test -v ./internal/database/...
go test -v ./internal/bot/...
```

---

## 🐧 12. Ubuntu & Linux Deployment (1 vCPU, 1 GB RAM)

### 📋 Supported Operating Systems:
| OS / Distro | Versi | Status |
| :--- | :--- | :--- |
| **Ubuntu LTS** | **24.04 LTS (Noble), 22.04 LTS (Jammy), 20.04 LTS (Focal)** | ✅ Direkomendasikan & Diuji |
| **Ubuntu Non-LTS**| **23.10, 24.10** | ✅ Didukung Penuh |
| **Debian** | **Debian 11 (Bullseye), Debian 12 (Bookworm)** | ✅ Didukung Penuh |
| **RHEL-based** | **Rocky Linux 8/9, AlmaLinux 8/9, CentOS Stream** | ✅ Didukung Penuh |

*(Karena menggunakan pure Go tanpa CGO/glibc lock-in, binary statis dapat berjalan di Linux kernel apa pun tanpa dependensi library eksternal).*

### A. Buat User Khusus (*Non-Root*)
```bash
sudo useradd -r -s /bin/false hostmonitor
```

### B. Siapkan Direktori Deployment
```bash
sudo mkdir -p /opt/host-monitor/data
sudo cp host-monitor /opt/host-monitor/
sudo cp .env.example /opt/host-monitor/.env
sudo nano /opt/host-monitor/.env

sudo chown -R hostmonitor:hostmonitor /opt/host-monitor
sudo chmod 700 /opt/host-monitor/data
sudo chmod 600 /opt/host-monitor/.env
sudo chmod 755 /opt/host-monitor/host-monitor
```

---

## ⚙️ 13. Systemd Service

Salin unit file hardening ke systemd:
```bash
sudo cp systemd/host-monitor.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now host-monitor
```

Cek status service:
```bash
sudo systemctl status host-monitor
```

---

## 🔥 14. Firewall (UFW)
Aplikasi menggunakan **Telegram Long Polling** (outbound HTTPS), sehingga **TIDAK MEMERLUKAN** port inbound terbuka:
```bash
sudo ufw default deny incoming
sudo ufw default allow outgoing
sudo ufw allow 22/tcp comment 'SSH'
sudo ufw enable
```

---

## 🔍 15. Troubleshooting
* **Bot tidak merespons pesan Telegram**:
  - Periksa apakah `TELEGRAM_BOT_TOKEN` valid.
  - Periksa apakah User ID pengirim sudah didaftarkan pada `ALLOWED_TELEGRAM_USER_IDS`.
  - Cek log aplikasi: `sudo journalctl -u host-monitor -f`.
* **Database error / Permission denied**:
  - Pastikan folder `/opt/host-monitor/data` dimiliki oleh user `hostmonitor`: `sudo chown -R hostmonitor:hostmonitor /opt/host-monitor/data`.
* **Koneksi target timeout**:
  - Periksa apakah server target memblokir IP VPS Anda atau port target tertutup firewall eksternal.

---

## 🔒 16. Security Considerations
* **Secret Protection**: Token bot tidak pernah dicatat di log aplikasi (*never logged*).
* **Zero Shell Execution**: Menghilangkan celah *Remote Code Execution* (RCE).
* **Isolation**: Pembatasan hak akses file SQLite (`chmod 600`) dan direktori data (`chmod 700`).

---

## 📊 17. Resource Requirements
* **CPU**: 1 vCPU (< 2% CPU usage saat idle & monitoring cycle).
* **RAM**: 1 GB RAM (Alokasi memory aplikasi ~15 - 25 MB RAM).
* **Storage**: < 50 MB disk space untuk binary dan SQLite database.
