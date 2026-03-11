# Primus Lens Web

GPU monitoring and analysis UI for the Primus platform.

Lens is deployed under the `/lens/` base path.

See the root [README](../../README.md) for setup instructions.

## Docker

```sh
docker build -t primus-lens-web:latest .
docker run -p 8080:80 primus-lens-web:latest
# Visit http://localhost:8080/lens
```
