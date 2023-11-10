# xlite-reverse-proxy
Reverse proxy daemon to use as endpoint for xlite-daemon \
Script create a http server listening on 0.0.0.0:11111 \
Then relay clients requests to servers in list set in configuration. \
default use static servers list, set in "var_defs.go", "serversJsonList". \
Endpoint can then be used for xlite-daemon and will relay calls/answers to servers passing the checks.

optional launch parameter:

    "-dynlist=true" use dynamic servers list parsed from "https://utils.blocknet.org/xrs/xrshowconfigs" 
#todo: P2P access to this call 

# Build instructions:
```
git clone https://github.com/tryiou/xlite-reverse-proxy.git
cd xlite-reverse-proxy
go get
# run with:
go run *.go

# build with:
# Linux
go build -o build/xlite-reverse-proxy_linux .
# Windows
go build -o build/xlite-reverse-proxy_windows.exe .
# macOS
go build -o build/xlite-reverse-proxy_macos .

# run bin with:
# Linux
./build/xlite-reverse-proxy_linux
# Windows
.\build\xlite-reverse-proxy_windows.exe
# macOS
./build/xlite-reverse-proxy_macos
```
 
