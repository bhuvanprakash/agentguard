# Contributing to AgentGuard

Thank you for your interest in contributing! We are building the industry's first open-source security layer for AI agents, and every contribution counts.

## 🌈 Code of Conduct

Please follow the Contributor Covenant in all interactions.

## 🛠️ Development Setup

### Prerequisites
- Go 1.22+
- Docker (optional)
- Postman / curl

### Setup
1. Fork the repository.
2. Clone your fork: `git clone https://github.com/YOUR_USERNAME/agentguard.git`
3. Run tests: `make test`
4. Run locally: `make run`

## 🏗️ Project Structure
- `proxy/`: The core interception engine.
- `policy/`: Rule evaluation and YAML parsing.
- `auth/`: HMAC signing and agent registration.
- `sdk/`: Client libraries for Python and Node.js.
- `audit/`: SQLite and Supabase log sync.

## 🧪 Testing Guidelines
- Add tests for new features in `_test.go` files.
- Run `make test` before opening a Pull Request.
- Run `./scripts/smoke-test.sh` to verify end-to-end functionality.

## 📄 Pull Request Process
1. Use descriptive commit messages.
2. Update documentation if you added/changed features.
3. Your PR must pass all CI checks.
4. One of the maintainers will review and merge.

## 💬 Community
- **GitHub Issues**: For bug reports and feature requests.
- **Discord**: Join our community for real-time discussion.

---
Happy coding! 🛡️
