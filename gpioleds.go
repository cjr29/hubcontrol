package main

import (
	"fmt"
	"os"

	"periph.io/x/conn/v3/gpio"
	"periph.io/x/conn/v3/gpio/gpioreg"
)

type LEDGPIO struct {
	Name    string
	PinName string
	Pin     gpio.PinIO // structure representing the GPIO line
	State   bool       // true = on, false = off
}

// Create a new LED structure given a GPIO chip, line name, and logging options

func NewGPIOLED(name string, PinName string) *LEDGPIO {
	light := new(LEDGPIO)
	light.Name = name
	light.PinName = PinName
	light.Pin = gpioreg.ByName(PinName)
	if light.Pin == nil {
		logger.Error(fmt.Sprintf("Failed to find GPIO line %s for LED", PinName))
		os.Exit(1)
	}
	// Set the pin as output initially Off.
	if err := light.Pin.Out(false); err != nil {
		logger.Error(fmt.Sprintf("Error setting GPIO pin. %v", err))
		os.Exit(1)
	}
	light.State = false
	light.Off()

	if Logging {
		logger.Debug(fmt.Sprintf("Created GPIO LED %s on line %s", light.Name, light.PinName))
	}

	return light
}

func (led *LEDGPIO) On() {
	led.Pin.Out(true)
	led.State = true
}

func (led *LEDGPIO) Off() {
	led.Pin.Out(false)
	led.State = false
}

func (led *LEDGPIO) Toggle() {
	if led.State {
		led.Off()
	} else {
		led.On()
	}
}
