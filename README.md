# xlite-reverse-proxy
Reverse proxy daemon to use as endpoint for xlite-daemon
script create a http server and relay client request to servers in list set in configuration.
default use static servers list, set in "var_defs.go", "serversJsonList".

optional launch parameter "-dynlist=true" to use dynamic servers list parsed from "https://utils.blocknet.org/xrs/xrshowconfigs"
#todo: P2P access to this call 

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
 
