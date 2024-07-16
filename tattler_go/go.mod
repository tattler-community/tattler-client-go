module tattler_go

go 1.22

replace github.com/tattler-community/tattler-client-go/fscache => ../fscache

require (
	github.com/kataras/golog v0.1.8
	github.com/tattler-community/tattler-client-go/fscache v0.0.0-00010101000000-000000000000
)

require (
	github.com/kataras/pio v0.0.11 // indirect
	golang.org/x/sys v0.0.0-20220919091848-fb04ddd9f9c8 // indirect
)
