# 🚀 RepoWatcher

**RepoWatcher** is a lightweight, efficient Go-based tool designed to monitor GitHub repositories for new commits and automatically restart your services seamlessly.

---

## 🌟 Features

- **Clone Public & Private Repositories:** Easily monitor any GitHub repository, whether public or private.
- **Customizable Polling:** Configure the interval for checking new commits according to your needs.
- **Graceful Restarts:** Execute custom shell commands to gracefully stop and restart your services.
- **Webhook Notifications:** Receive instant notifications via Slack, Telegram, or Discord.
- **Structured Logging:** Maintain clear and organized logging directly to stdout/stderr.
- **Graceful Shutdown:** Cleanly handles SIGINT and SIGTERM signals.

---

## 📦 Installation

To get started quickly, clone the repository and build the tool:

```bash
git clone https://github.com/Aswin-ElMagnifico/repo-watcher.git
cd repo-watcher
go build -o repo-watcher main.go
```

---

## ⚙️ Usage

Customize and run `repo-watcher` with your configuration:

```bash
./repo-watcher --config=config.yml
```

*(See example `config.yml` for setup details.)*

---

## 📖 Configuration Example

Here's a quick look at a sample configuration file:

```yaml
repo_url: https://github.com/yourname/yourrepo.git
auth_token: YOUR_PAT_IF_NEEDED
poll_interval_seconds: 60
branch: main
notifier:
  type: slack
  webhook_url: https://hooks.slack.com/services/XXX/YYY/ZZZ
```

---

## 📝 License

RepoWatcher is released under the MIT License. See [LICENSE](LICENSE) for details.

---

**Enjoy effortless repository monitoring! 🎉**

