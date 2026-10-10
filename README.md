# 🖥️ Host Monitoring Telegram Bot (Golang)

Personal lightweight, secure, and production-ready Host Monitoring Telegram Bot built with Go and SQLite. Designed specifically for low-resource environments (Ubuntu Linux VPS with 1 vCPU and 1 GB RAM).

---

## 📌 1. Project Overview
Bot Telegram mandiri untuk memantau status ketersediaan (*availability*), *latency*, respons protokol HTTP/HTTPS, dan port TCP pada **domain, public IPv4, dan public IPv6**. Bot berjalan secara otomatis di latar belakang dan mengirimkan notifikasi instan ke Telegram hanya ketika terjadi perubahan status (misal: `ONLINE -> DOWN` atau `DOWN -> RECOVERED`).

---

## 🚀 2. Features
* **Zero Bloat & Lightweight**: 100% menggunakan Go Standard Library & SQLite (CGO-free). Memori runtime < 25 MB RAM.
* **Smart Target Detection**: Otomatis mendeteksi tipe target (`DOMAIN`, `IPV4`, `IPV6`) baik dengan port standar maupun **Custom Port**.
* **Custom Port Monitoring per Target**: Mendukung monitoring port khusus seperti database (`:3306`, `:5432`), web API (`:8080`, `:8443`), Redis (`:6379`), dsb.
* **Interactive Menu & Telegram Buttons**: Dilengkapi tombol **[ Menu ]** bawaan Telegram (`setMyCommands`) dan tombol klik instan (*Reply Keyboard*) tanpa perlu mengetik manual.
* **Multi-Protocol Probing**:
  - **Domain**: DNS resolution (A & AAAA records), HTTP (Port 80/Custom), HTTPS (Port 443/Custom dengan validasi TLS), TCP Port, Latency.
  - **IP (v4 & v6)**: TCP Probe port (*non-root* ICMP fallback & reachability), TCP Port checks (default 22, 80, 443 atau custom port target).
* **Robust Security & SSRF Protection**: Menolak seluruh rentang IP privat/internal, loopback, link-local metadata cloud (`169.254.169.254`), dan proteksi *DNS rebinding*.
* **Status-Change Alerting (No Spam)**: Hanya mengirim notifikasi saat status berubah (`ONLINE -> DOWN` atau `DOWN -> ONLINE`).
* **Resource Boundary Guard**: Batas goroutine concurrent, timeout ketat per koneksi jaringan, pembatasan body response, dan batasan redirect.
* **Telegram User Whitelist & Rate Limiter**: Akses terbatas hanya untuk Telegram User ID yang terdaftar dengan pembatasan frekuensi perintah (*sliding-window rate limit*).
* **Automatic History & Retention**: Menyimpan riwayat pemeriksaan ke SQLite dengan pembersihan log otomatis berkala (*retention policy*).
* **Uptime Report & SLA Compliance**: Menghitung persentase ketersediaan (*uptime percentage*), durasi gangguan (*downtime*), rata-rata latensi, dan status evaluasi SLA (`99.9%`, `99.0%`) dalam rentang waktu harian/mingguan/bulanan (`/uptime` atau `/sla`).
* **Pure Go WHOIS & Domain Expiry**: Mengecek masa aktif, tanggal kedaluwarsa (*expiry date*), status registrar, name server, dan status ketersediaan domain `.id` (PANDI) maupun domain luar/internasional secara langsung via TCP Port 43 tanpa butuh API eksternal (`/whois <domain>`).

---

## 🎯 3. Supported Target
| Tipe | Contoh Input Standar | Contoh Input Custom Port | Pemeriksaan |
| :--- | :--- | :--- | :--- |
| **Domain** | `example.com`, `google.com` | `example.com:8080`, `api.mysite.com:8443` | DNS Resolution, HTTP/HTTPS Custom Port, TCP Port, Latency |
| **IPv4** | `123.123.123.123`, `1.1.1.1` | `103.12.34.56:3306` (MySQL), `1.2.3.4:5432` | TCP Custom Port Probe, Reachability, Latency |
| **IPv6** | `2001:db8::1` | `[2001:db8::1]:8443` | TCP Custom Port Probe, Reachability, Latency |

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

Clone repository dari GitHub:
```bash
git clone https://github.com/iqbalmusyaffa/bot-monitoring-server.git
cd bot-monitoring-server
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

## 💬 9. Commands & Interactive Keyboard

Bot dilengkapi dengan papan tombol interaktif (*Persistent Reply Keyboard*) dan tombol **[ Menu ]** otomatis bawaan Telegram (`setChatMenuButton`), sehingga pengguna dapat mengeklik tombol menu langsung tanpa perlu mengetik garis miring `/`:

```text
+-----------------------+-----------------------+
|    📋 Daftar Host     |    📈 Uptime & SLA    |
+-----------------------+-----------------------+
|    ⚡ Cek Cepat       |    🌐 Cek WHOIS       |
+-----------------------+-----------------------+
|    ➕ Tambah Host     |    📊 Status Host     |
+-----------------------+-----------------------+
|    👥 Kelola User     |    ❓ Panduan         |
+-----------------------+-----------------------+
```
*(Catatan: Tombol `➕ Tambah Host` dan `👥 Kelola User` disesuaikan otomatis dengan hak akses Role Anda. Tombol keyboard ini dapat disembunyikan/dimunculkan kapan saja lewat ikon `[::]` di samping emoticon Telegram).*

### 👑 Daftar Perintah Berdasarkan Role:

| Perintah | Contoh | Hak Akses | Deskripsi |
| :--- | :--- | :--- | :--- |
| `/start` | `/start` | Semua Role | Menampilkan menu utama & memunculkan papan tombol interaktif |
| `/help` | `/help` | Semua Role | Menampilkan panduan penggunaan lengkap |
| `/check <host>` | `/check example.com` | Semua Role | Mengecek kondisi host secara instan (DNS, HTTP/S, Port, Latency) |
| `/list` | `/list` | Semua Role | Menampilkan daftar seluruh host yang sedang dimonitor |
| `/status <host>` | `/status example.com` | Semua Role | Melihat status terakhir, response time & riwayat di database |
| `/uptime [host] [durasi]` | `/uptime` atau `/uptime example.com 7d` | Semua Role | Laporan persentase uptime & evaluasi SLA (`24h`, `7d`, `30d`) |
| `/whois <domain>` | `/whois kompas.id` atau `/whois google.com` | Semua Role | Cek masa aktif, tanggal expired, registrar & status domain |
| `/add <host>` | `/add 123.123.123.123` atau `/add site.com:8080` | **Admin / Owner** | Menambahkan target baru ke monitoring berkala otomatis |
| `/remove <host>` | `/remove example.com` | **Admin / Owner** | Menghapus target dari daftar monitoring otomatis |
| `/monitor <host>` | `/monitor example.com` | **Admin / Owner** | Mengaktifkan kembali monitoring otomatis untuk target yang dijeda |
| `/unmonitor <host>`| `/unmonitor example.com` | **Admin / Owner** | Menjeda (*pause*) monitoring otomatis untuk target |
| `/listusers` | `/listusers` | **Owner Only** | Melihat seluruh daftar pengguna terdaftar beserta rolenya |
| `/adduser <id> [role]` | `/adduser 987654321 admin` | **Owner Only** | Mendaftarkan pengguna baru dengan hak akses `admin` atau `user` |
| `/removeuser <id>` | `/removeuser 987654321` | **Owner Only** | Mencabut akses pengguna terdaftar |

---

## 📊 10. Showcase Fitur Unggulan

### 📈 A. Laporan Uptime & Evaluasi SLA (`/uptime` atau `/sla`)
Menghitung persentase ketersediaan (*availability percentage*) host dari database SQLite dalam kurun waktu tertentu:
* **Sintaks**:
  * `/uptime` : Ringkasan performa seluruh host aktif dalam 24 jam terakhir.
  * `/uptime 7d` : Ringkasan seluruh host dalam 7 hari terakhir (mendukung `24h`, `48h`, `7d`, `14d`, `30d`).
  * `/uptime <host> [durasi]` : Laporan komprehensif untuk satu host tertentu (misal: `/uptime example.com 7d`).
* **Standar Evaluasi SLA (Tiering)**:
  * `✅ PASSED (Tier 99.9%)` : Uptime $\ge 99.9\%$ (Standar ketersediaan tinggi / *high availability*).
  * `⚠️ WARNING (Tier 99.0%)` : Uptime antara $99.0\% - 99.89\%$ (Terjadi beberapa insiden *downtime*).
  * `❌ BREACHED (< 99.0%)` : Uptime $< 99.0\%$ (Pelanggaran batas toleransi gangguan).

```text
Contoh Output Detail (/uptime example.com 7d):
-------------------------------------------------
📈 LAPORAN UPTIME & SLA (7 Hari Terakhir)
Target: example.com (DOMAIN)

Status Terkini: 🟢 ONLINE

⏱️ Uptime: 99.92%
🎯 Status SLA: ✅ PASSED (Tier 99.9%)
⚡ Rata-rata Latensi: 38 ms
🔄 Total Pemeriksaan: 10,080 kali
✅ Sukses (ONLINE): 10,072 kali
📉 Gangguan (DOWN): 8 kali (~8m0s)

Terakhir Dicek: 10 Okt 2026 15:00:00 WIB
Terakhir Online: 10 Okt 2026 15:00:00 WIB
```

---

### 🌐 B. Cek WHOIS & Masa Aktif Domain (`/whois` atau `/domain`)
Pengecekan tanggal kedaluwarsa registrasi domain tanpa API berbayar (100% Pure Go TCP Port 43):
* **Cakupan Luas**: Otomatis mengenali domain Indonesia ccTLD `.id` (termasuk `.co.id`, `.web.id`, `.my.id`, dll. via PANDI) serta gTLD internasional (`.com`, `.net`, `.org`, `.io`, `.xyz`, dll.).
* **4 Mode Tampilan Cerdas**:
  1. `🟢 AKTIF (NORMAL)` : Domain aktif dengan sisa waktu aman ($> 30$ hari).
  2. `🟡 PERINGATAN (H-30)` : Mengingatkan pemilik bahwa domain akan habis dalam $< 30$ hari.
  3. `🚨 KRITIS (H-7)` : Peringatan darurat bahwa domain akan habis dalam $\le 7$ hari.
  4. `🚨 KEDALUWARSA (EXPIRED)` : Tampilan merah tegas dengan instruksi perpanjangan darurat sebelum masuk masa *Pending Delete*.
  5. `🟢 TERSEDIA (AVAILABLE)` : Memberi tahu bahwa domain belum didaftarkan dan dapat dibeli.

```text
Contoh Output Domain Kedaluwarsa (/whois expired-site.id):
-----------------------------------------------------------
🚨 STATUS: DOMAIN SUDAH KEDALUWARSA (EXPIRED) 🚨
Target: expired-site.id

🔴 Kondisi         : ❌ MATI / KEDALUWARSA
⏳ Tanggal Expired : 15 Sep 2026 07:00 WIB
⏱️ Telah Lewat     : 25 hari yang lalu

🏢 Registrar       : PT Rumahweb Indonesia
📅 Awal Terdaftar  : 15 Sep 2021
🔒 Status EPP      : redemptionPeriod, pendingDelete

🌐 Name Servers:
   • ns1.expired-site.id
   • ns2.expired-site.id

⚠️ PERHATIAN:
Domain ini telah melewati masa tenggang. Segera lakukan perpanjangan (renew) sebelum masuk ke siklus lelang / pendingDelete!
```

---

## 💻 11. Local Development
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

## 🧪 12. Testing
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

## 🐧 13. Panduan Lengkap Deployment di VPS Ubuntu (1 vCPU, 1 GB RAM)

### 📋 Supported Operating Systems:
| OS / Distro | Versi | Status |
| :--- | :--- | :--- |
| **Ubuntu LTS** | **24.04 LTS (Noble), 22.04 LTS (Jammy), 20.04 LTS (Focal)** | ✅ Direkomendasikan & Diuji |
| **Ubuntu Non-LTS**| **23.10, 24.10** | ✅ Didukung Penuh |
| **Debian** | **Debian 11 (Bullseye), Debian 12 (Bookworm)** | ✅ Didukung Penuh |
| **RHEL-based** | **Rocky Linux 8/9, AlmaLinux 8/9, CentOS Stream** | ✅ Didukung Penuh |

*(Karena menggunakan pure Go tanpa CGO/glibc lock-in, binary statis dapat berjalan di Linux kernel apa pun tanpa dependensi library eksternal).*

---

### 🚀 Cara 1: Install & Build Langsung di VPS Ubuntu (Recommended)

1. **Update package & install Git + Go (di VPS):**
   ```bash
   sudo apt update && sudo apt install -y git golang-go
   ```

2. **Clone repo, unduh dependencies & build binary:**
   ```bash
   git clone https://github.com/iqbalmusyaffa/bot-monitoring-server.git
   cd bot-monitoring-server
   go mod tidy
   go build -ldflags="-s -w" -o host-monitor cmd/bot/main.go
   ```

3. **Buat user khusus (*non-root*):**
   ```bash
   sudo useradd -r -s /bin/false hostmonitor
   ```

4. **Siapkan direktori aplikasi `/opt/host-monitor`:**
   ```bash
   sudo mkdir -p /opt/host-monitor/data
   sudo cp host-monitor /opt/host-monitor/
   sudo cp .env.example /opt/host-monitor/.env
   sudo chown -R hostmonitor:hostmonitor /opt/host-monitor
   sudo chmod 700 /opt/host-monitor/data
   sudo chmod 600 /opt/host-monitor/.env
   sudo chmod 755 /opt/host-monitor/host-monitor
   ```

5. **Isi Token Telegram di file `.env`:**
   ```bash
   sudo nano /opt/host-monitor/.env
   ```
   *(Cukup isi `TELEGRAM_BOT_TOKEN=...`, lalu simpan dengan `Ctrl+O`, `Enter`, `Ctrl+X`)*.

6. **Pasang & jalankan Systemd Service:**
   ```bash
   sudo cp systemd/host-monitor.service /etc/systemd/system/
   sudo systemctl daemon-reload
   sudo systemctl enable --now host-monitor
   ```

7. **Cek status & log:**
   ```bash
   sudo systemctl status host-monitor
   sudo journalctl -u host-monitor -f
   ```

8. **Klaim Owner:** Buka bot Anda di Telegram dan ketik `/start`. Anda otomatis menjadi Owner! 🎉

---

### 💻 Cara 2: Cross-Compile dari Komputer Lokal (Windows / macOS)

Jika Anda tidak ingin menginstall Go di VPS:

1. **Build binary Linux di komputer lokal:**
   - **Windows (PowerShell):**
     ```powershell
     $env:GOOS="linux"; $env:GOARCH="amd64"; $env:CGO_ENABLED="0"; go build -ldflags="-s -w" -o host-monitor ./cmd/bot/main.go
     ```
   - **macOS / Linux:**
     ```bash
     GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o host-monitor ./cmd/bot/main.go
     ```

2. **Upload binary & config ke VPS via SCP:**
   ```bash
   scp host-monitor .env.example systemd/host-monitor.service user@IP_VPS:~
   ```

3. **Login ke VPS dan ikuti langkah pembuatan user & direktori seperti pada Cara 1.**

---

## 🔄 14. Cara Update & Redeploy Versi Terbaru di VPS

Saat ada pembaruan kode baru dari GitHub, ikuti langkah berikut untuk mengupdate bot di VPS tanpa kehilangan data konfigurasi maupun histori database:

### Opsi A: Update Langsung via Git di VPS (Paling Praktis)
```bash
# 1. Masuk ke direktori git repository
cd ~/bot-monitoring-server

# 2. Reset perubahan lokal & tarik update terbaru dari GitHub
git reset --hard HEAD
rm -f go.sum
git pull

# 3. Compile ulang binary terbaru
go build -ldflags="-s -w" -o host-monitor cmd/bot/main.go

# 4. Hentikan service sebentar (wajib agar tidak error 'Text file busy')
sudo systemctl stop host-monitor

# 5. Salin binary baru & atur hak akses
sudo cp host-monitor /opt/host-monitor/host-monitor
sudo chown hostmonitor:hostmonitor /opt/host-monitor/host-monitor
sudo chmod 755 /opt/host-monitor/host-monitor

# 6. Jalankan kembali service bot
sudo systemctl start host-monitor

# 7. Pastikan status bot berjalan normal
sudo systemctl status host-monitor
```

### Opsi B: Build dari Laptop Windows & Upload via SCP
```powershell
# 1. Di PowerShell Laptop:
$env:GOOS="linux"; $env:GOARCH="amd64"; $env:CGO_ENABLED="0"; go build -ldflags="-s -w" -o host-monitor ./cmd/bot/main.go
scp host-monitor root@IP_VPS:/opt/host-monitor/host-monitor-new

# 2. Di Terminal VPS:
sudo systemctl stop host-monitor
sudo mv /opt/host-monitor/host-monitor-new /opt/host-monitor/host-monitor
sudo chown hostmonitor:hostmonitor /opt/host-monitor/host-monitor
sudo chmod 755 /opt/host-monitor/host-monitor
sudo systemctl start host-monitor
sudo systemctl status host-monitor
```

---

## 📋 15. Pemeliharaan, Monitoring Log & Backup Database

### 🔍 Melihat Log Bot (`journalctl`):
* **Live Streaming Log Realtime (Follow):**
  ```bash
  sudo journalctl -u host-monitor -f
  ```
  *(Tekan `Ctrl + C` untuk keluar dari log)*.

* **Melihat 50 baris log terakhir:**
  ```bash
  sudo journalctl -u host-monitor -n 50 --no-pager
  ```

* **Melihat log sejak restart terakhir:**
  ```bash
  sudo journalctl -u host-monitor -b --no-pager
  ```

* **Filter log hanya yang berisi error:**
  ```bash
  sudo journalctl -u host-monitor -p err --no-pager
  ```

### 💾 Backup & Restore Database SQLite:
File database SQLite terletak di `/opt/host-monitor/data/monitor.db`.

* **Backup Database Aman:**
  ```bash
  sudo cp /opt/host-monitor/data/monitor.db /opt/host-monitor/data/monitor_backup_$(date +%F).db
  ```
* **Restore Database:**
  ```bash
  sudo systemctl stop host-monitor
  sudo cp /path/to/backup.db /opt/host-monitor/data/monitor.db
  sudo chown hostmonitor:hostmonitor /opt/host-monitor/data/monitor.db
  sudo chmod 600 /opt/host-monitor/data/monitor.db
  sudo systemctl start host-monitor
  ```

---

## ⚙️ 16. Systemd Service

Salin unit file hardening ke systemd:
```bash
sudo cp systemd/host-monitor.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now host-monitor
```

Perintah manajemen service:
```bash
sudo systemctl start host-monitor     # Menjalankan bot
sudo systemctl stop host-monitor      # Menghentikan bot
sudo systemctl restart host-monitor   # Merestart bot
sudo systemctl status host-monitor    # Cek status bot
```

---

## 🔥 17. Firewall (UFW)
Aplikasi menggunakan **Telegram Long Polling** (outbound HTTPS ke api.telegram.org) dan TCP probe outbound, sehingga **TIDAK MEMERLUKAN** port inbound terbuka:
```bash
sudo ufw default deny incoming
sudo ufw default allow outgoing
sudo ufw allow 22/tcp comment 'SSH'
sudo ufw enable
```

---

## 🔍 18. Troubleshooting
* **Error `Text file busy` saat copy binary:**
  - Terjadi karena file binary lama sedang dieksekusi oleh service. Hentikan service terlebih dahulu: `sudo systemctl stop host-monitor`, lalu ulangi copy.
* **Error `Your local changes would be overwritten by merge` saat git pull:**
  - Bersihkan modifikasi file lokal sementara di VPS: `git reset --hard HEAD && rm -f go.sum && git pull`.
* **Tombol Menu tidak muncul di Telegram Desktop:**
  - Ketik `/start` sekali di chat bot. Jika keyboard sempat tertutup, klik ikon 4 kotak kecil `[::]` di pojok kanan kolom input (sebelah kiri emoticon 😊) untuk membukanya kembali.
* **Bot tidak merespons pesan Telegram:**
  - Periksa apakah `TELEGRAM_BOT_TOKEN` valid di file `.env`.
  - Periksa apakah User ID pengirim sudah terdaftar di database via `/listusers` atau `.env`.
  - Cek log aplikasi: `sudo journalctl -u host-monitor -f`.
* **Database error / Permission denied:**
  - Pastikan kepemilikan direktori data sudah benar: `sudo chown -R hostmonitor:hostmonitor /opt/host-monitor/data`.
* **Koneksi target timeout:**
  - Periksa apakah server target memblokir IP VPS Anda atau port target tertutup firewall eksternal.

---

## 🔒 19. Security Considerations
* **Secret Protection**: Token bot tidak pernah dicatat di log aplikasi (*never logged*).
* **Zero Shell Execution**: 100% Go native socket & HTTP library, menghilangkan celah *Remote Code Execution* (RCE).
* **Isolation**: Pembatasan hak akses file SQLite (`chmod 600`) dan direktori data (`chmod 700`).
* **Non-Root Sandboxing**: Systemd hardening dengan `NoNewPrivileges=true`, `ProtectSystem=strict`, `ProtectHome=true`.

---

## 📊 20. Resource Requirements
* **CPU**: 1 vCPU (< 2% CPU usage saat idle & monitoring cycle).
* **RAM**: 1 GB RAM (Alokasi memory aplikasi ~15 - 25 MB RAM).
* **Storage**: < 50 MB disk space untuk binary dan SQLite database.
