// Test-only identity simulator. It is not embedded in or required by fastems.
// It exposes a YD2037-like ASCII model string at holding register 43904.
package main

import (
	"flag"
	"log"

	"github.com/simpleiot/simpleiot/modbus"
)

func main() {
	port := flag.String("port", "1503", "listen port")
	id := flag.Int("id", 1, "Modbus server ID")
	text := flag.String("text", "YD2037Y-F", "ASCII identity text")
	address := flag.Int("address", 43904, "holding register address")
	flag.Parse()
	regs := &modbus.Regs{}
	regs.AddReg(0, 1)
	words := make([]uint16, 0, 16)
	for i := 0; i < len(*text); i += 2 {
		high := uint16((*text)[i])
		low := uint16(' ')
		if i+1 < len(*text) {
			low = uint16((*text)[i+1])
		}
		words = append(words, high<<8|low)
	}
	for len(words) < 16 {
		words = append(words, 0x2020)
	}
	regs.AddReg(*address, len(words))
	for i, word := range words {
		if err := regs.WriteReg(*address+i, word); err != nil {
			log.Fatal(err)
		}
	}
	server, err := modbus.NewTCPServer(*id, 10, *port, regs, 0)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("TEST identity simulator :%s id=%d %d@%d=%q", *port, *id, uint16(*address), *address, *text)
	server.Listen(func(err error) { log.Print(err) }, func() {}, func() {})
}
