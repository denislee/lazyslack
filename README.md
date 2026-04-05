# lazyslack

**lazyslack** is a lightweight, high-performance Terminal User Interface (TUI) client for Slack. Built with Go and the [Bubble Tea](https://github.com/charmbracelet/bubbletea) framework, it provides a fast, keyboard-centric experience for staying connected on Slack without leaving your terminal.

![lazyslack Demo](https://raw.githubusercontent.com/user/lazyslack/main/docs/demo.png) *(Placeholder for demo image)*

## ✨ Features

- **Keyboard-Centric**: Navigate through channels, threads, and messages using Vim-inspired keybindings.
- **Rich TUI Experience**: Interactive components for channel lists, message threading, status bars, and help overlays.
- **Full Slack Integration**: Support for public channels, private channels, direct messages (DMs), and multi-person DMs (MPIMs).
- **Advanced UI Components**:
  - **Avatars**: Beautifully rendered half-block terminal avatars.
  - **Thread Support**: Easily view and reply to threaded conversations.
  - **Emoji Reactions**: React to messages or remove existing reactions.
  - **Quick Switcher & Search**: Navigate quickly between channels or search for specific content.
- **Efficient Caching**: Local message and user metadata caching for a snappy response time.
- **Customizable**: Configurable through environment variables or a TOML file.

## 🚀 Getting Started

### Prerequisites

- **Go**: Version 1.25.0 or higher.
- **Slack API Token**: You'll need a [Slack app token](https://api.slack.com/authentication/token-types#bot) (usually starting with `xoxb-`) with appropriate scopes (`channels:read`, `groups:read`, `im:read`, `mpim:read`, `chat:write`, `reactions:write`, etc.).

### Installation

Clone the repository and build the executable:

```bash
git clone https://github.com/user/lazyslack.git
cd lazyslack
go build -o lazyslack ./cmd/lazyslack
```

### Running

Set your Slack token and run the application:

```bash
SLACK_TOKEN="xoxb-your-token-here" ./lazyslack
```

## ⚙️ Configuration

`lazyslack` can be configured via environment variables or a TOML configuration file.

### Environment Variables

- `SLACK_TOKEN` (**Required**): Your Slack authentication token.
- `SLACK_TEAM` (Optional): Team ID to filter the workspace.
- `LAZYSLACK_CONFIG` (Optional): Path to a custom TOML configuration file.

### Configuration File

By default, the application looks for a configuration at `~/.config/lazyslack/config.toml`.

Example `config.toml`:

```toml
[display]
message_limit = 100
theme = "dark"
emoji_style = "native"

[polling]
active_channel_interval = "5s"
thread_interval = "10s"

[channels]
pinned = ["C12345678", "G87654321"]
types = ["public_channel", "private_channel", "im", "mpim"]
```

## ⌨️ Keybindings

| Key | Action |
| --- | --- |
| `j`/`↓` | Navigate down |
| `k`/`↑` | Navigate up |
| `i` | Enter compose mode |
| `r` | Reply to message (Thread) |
| `enter` | Open channel / thread |
| `esc` | Back / Close modal |
| `+` | Add reaction |
| `-` | Remove reaction |
| `/` | Search |
| `y` | Yank (Copy) message link/text |
| `u` | Filter for unread messages only |
| `v` | Toggle sidebar layout |
| `?` | Show help |
| `ctrl+c` | Quit |

## 🛠️ Architecture

The project follows a modular architecture:

- `cmd/lazyslack/`: Main entry point and dependency wiring.
- `internal/slack/`: Slack API client abstraction, caching, and mapping logic.
- `internal/ui/`: Core TUI logic using Bubble Tea.
  - `component/`: Reusable UI elements (Avatars, Message lists, Status bar).
  - `screen/`: Main application views (Chat, Channels, Thread).
- `internal/config/`: Configuration and state management.

## 🤝 Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

1. Fork the project.
2. Create your feature branch (`git checkout -b feature/AmazingFeature`).
3. Commit your changes (`git commit -m 'Add some AmazingFeature'`).
4. Push to the branch (`git push origin feature/AmazingFeature`).
5. Open a Pull Request.

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
