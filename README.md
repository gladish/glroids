Asteroids clone in Go (100% AI generated)

Live (WASM): https://gladish.github.io/glroids/

# WASM Build/Run
This is powershell syntax. If you're in another shell...

* Compile .wasm target

```$env:GOOS="js"; $env:GOARCH="wasm"; go build -o glroids.wasm .```

* Copy the wasm js from go installation.

```Copy-Item 'C:\Program Files\Go\lib\wasm\wasm_exec.js' .```

* Run webserver

```python -m http.server 8080```
