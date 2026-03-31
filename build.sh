go build -o fins cmd/fins/main.go
go build -o finsd cmd/finsd/main.go
pkill finsd; ./finsd > finsd.log 2>&1 &
sudo cp fins /usr/local/bin/fins
sudo cp finsd /usr/local/bin/finsd