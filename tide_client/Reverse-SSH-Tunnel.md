## Setting Up a Reverse SSH Tunnel Using systemd

This guide explains how to establish a reverse SSH tunnel from an internal (private network) machine to a public-facing server using `systemd`. Instead of relying on `autossh`, we’ll use a regular `ssh` command, with `systemd` responsible for maintaining and restarting the connection.

---

### 🧰 Prerequisites

- The internal machine must be able to initiate outbound SSH connections.
- The public server must have SSH access enabled and allow remote port forwarding.
- SSH key-based authentication must be set up from the internal machine (`navitech` user) to the remote server user (`pi@192.168.1.2`).

---

### 🔐 Step 1: Set Up SSH Key Authentication (if not done already)

Run the following on the internal machine:

```bash
ssh-keygen -t ed25519 -f ~/.ssh/SIO
ssh-copy-id -i ~/.ssh/SIO pi@192.168.1.2
```

This will enable passwordless login using the SSH key.

---

### 🛠 Step 2: Create a systemd Service File

Create a new service file at `/etc/systemd/system/ssh-reverse-tunnel.service`:

```bash
sudo nano /etc/systemd/system/ssh-reverse-tunnel.service
```

Paste the following content:

```ini
[Unit]
Description=SSH Reverse Tunnel Service
After=network.target

[Service]
User=navitech
ExecStart=/usr/bin/ssh -o ServerAliveInterval=60 -o ServerAliveCountMax=5 -o ExitOnForwardFailure=yes -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -N -T -R 60992:localhost:22 pi@192.168.1.2 -i /home/navitech/.ssh/SIO
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

---

### ⚙️ Step 3: Enable and Start the Service

Reload systemd, enable the service at boot, and start it:

```bash
sudo systemctl daemon-reload
sudo systemctl enable ssh-reverse-tunnel.service
sudo systemctl start ssh-reverse-tunnel.service
```

---

### ✅ Step 4: Check Service Status

Check if the service is running properly:

```bash
sudo systemctl status ssh-reverse-tunnel.service
```

You should see a status like `active (running)`.

---

### 🌐 Step 5: Access the Internal Machine from the Remote Server

On the remote server, connect to the internal machine through the reverse tunnel:

```bash
ssh -p 60992 navitech@localhost
```

If the `navitech` user exists on the internal machine, you should be able to log in directly.

---

### 🔎 Notes

- `-R 60992:localhost:22` opens port `60992` on the public server and forwards it to the SSH service on the internal machine.
- If you want external hosts to connect to this forwarded port, ensure that the remote server’s SSH config (`/etc/ssh/sshd_config`) includes:

  ```bash
  GatewayPorts yes
  ```

- Also, make sure any firewalls on the remote server allow traffic on the forwarded port.

---

Let me know if you'd like this as a downloadable file or want to include additional examples or tips (like using `~/.ssh/config`, port randomization, or multiple tunnels).