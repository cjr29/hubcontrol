package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"periph.io/x/conn/v3/gpio"
	"periph.io/x/conn/v3/gpio/gpioreg"
	"periph.io/x/host/v3"
)

const AppVersion = "1.0.0"

const (
	SLOW = 1.0
	FAST = 4.0
)

const (
	OFF int = iota
	REDON
	GREENON
	REDSLOW
	GREENSLOW
	REDFAST
	GREENFAST
)

const (
	defaultStateFile    = "/var/sensorhub/led.state"
	sensorHubConfigFile = "/var/sensorhub/sensorhub.conf"

	// Change these to match the GPIO connections.
	defaultRedGPIO    = "GPIO23"
	defaultGreenGPIO  = "GPIO24"
	defaultButtonGPIO = "GPIO25"

	configPollInterval = 250 * time.Millisecond
)

func (l *LEDGPIO) Close() {
	l.Off()

	if err := l.Pin.Halt(); err != nil {
		logger.Debug("%s: unable to release GPIO: %v", l.Name, err)
	}
}

type LightController struct {
	red   *LEDGPIO
	green *LEDGPIO

	currentState int

	blinkTicker *time.Ticker
	blinkLED    *LEDGPIO
	blinkC      <-chan time.Time
}

func NewLightController(red, green *LEDGPIO) *LightController {
	return &LightController{
		red:          red,
		green:        green,
		currentState: -1,
	}
}

func (c *LightController) stopBlinking() {
	if c.blinkTicker != nil {
		c.blinkTicker.Stop()
		c.blinkTicker = nil
		c.blinkC = nil
		c.blinkLED = nil
	}
}

func (c *LightController) startBlinking(led *LEDGPIO, rateHz float64) {
	if rateHz <= 0 {
		return
	}

	halfPeriod := time.Duration(float64(time.Second) / (2 * rateHz))
	if halfPeriod <= 0 {
		return
	}

	c.blinkLED = led
	c.blinkTicker = time.NewTicker(halfPeriod)
	c.blinkC = c.blinkTicker.C

	// Start with the LED off.
	led.Off()
}

func (c *LightController) SetState(state int) {
	if state == c.currentState {
		return
	}

	c.stopBlinking()

	// Every state starts with both LEDs off.
	c.red.Off()
	c.green.Off()

	switch state {
	case OFF:
		logger.Debug("LED state: OFF")

	case REDON:
		c.red.On()
		logger.Debug("LED state: REDON")

	case GREENON:
		c.green.On()
		logger.Debug("LED state: GREENON")

	case REDSLOW:
		c.startBlinking(c.red, SLOW)
		logger.Debug("LED state: REDSLOW")

	case GREENSLOW:
		c.startBlinking(c.green, SLOW)
		logger.Debug("LED state: GREENSLOW")

	case REDFAST:
		c.startBlinking(c.red, FAST)
		logger.Debug("LED state: REDFAST")

	case GREENFAST:
		c.startBlinking(c.green, FAST)
		logger.Debug("LED state: GREENFAST")

	default:
		logger.Debug(fmt.Sprintf("invalid LED state number: %d", state))
		return
	}

	c.currentState = state
}

func (c *LightController) HandleBlink() {
	if c.blinkLED == nil {
		return
	}

	c.blinkLED.Toggle()
}

func (c *LightController) Close() {
	c.stopBlinking()

	c.red.Off()
	c.green.Off()

	c.red.Close()
	c.green.Close()
}

var stateNames = map[string]int{
	"OFF":       OFF,
	"REDON":     REDON,
	"GREENON":   GREENON,
	"REDSLOW":   REDSLOW,
	"GREENSLOW": GREENSLOW,
	"REDFAST":   REDFAST,
	"GREENFAST": GREENFAST,
}

func parseState(value string) (int, error) {
	value = strings.ToUpper(strings.TrimSpace(value))

	if state, ok := stateNames[value]; ok {
		return state, nil
	}

	// Also allow numeric state values 0 through 6.
	number, err := strconv.Atoi(value)
	if err == nil && number >= OFF && number <= GREENFAST {
		return number, nil
	}

	return 0, fmt.Errorf("unknown LED state %q", value)
}

func readStateFile(path string) (int, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, "", err
	}

	raw := strings.TrimSpace(string(data))
	state, err := parseState(raw)
	if err != nil {
		return 0, raw, err
	}

	return state, raw, nil
}

// writeLEDState converts the integer state to its file representation.
func writeStateFile(state int) error {
	var stateText string
	switch state {
	case OFF:
		stateText = "OFF"
	case REDON:
		stateText = "REDON"
	case GREENON:
		stateText = "GREENON"
	case REDSLOW:
		stateText = "REDSLOW"
	case GREENSLOW:
		stateText = "GREENSLOW"
	case REDFAST:
		stateText = "REDFAST"
	case GREENFAST:
		stateText = "GREENFAST"
	default:
		return fmt.Errorf("invalid LED state: %d", state)
	}

	// The newline is optional; remove it if the receiving device does not
	// expect one.
	return os.WriteFile(
		defaultStateFile,
		[]byte(stateText+"\n"),
		0644,
	)
}

func run(ctx context.Context, controller *LightController, stateFile string) {
	configTicker := time.NewTicker(configPollInterval)
	defer configTicker.Stop()

	lastRaw := ""
	fileWasMissing := false

	// Start safely with both LEDs off.
	controller.SetState(OFF)

	for {
		select {
		case <-ctx.Done():
			return

		case <-configTicker.C:
			state, raw, err := readStateFile(stateFile)

			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					if !fileWasMissing {
						logger.Debug(fmt.Sprintf("state file %s is missing; turning LEDs off", stateFile))
						controller.SetState(OFF)
						fileWasMissing = true
						lastRaw = ""
					}
					continue
				}

				// Avoid repeatedly logging the same invalid value.
				if raw != lastRaw || fileWasMissing {
					logger.Debug(fmt.Sprintf("reading state file %s: %v", stateFile, err))
					lastRaw = raw
					fileWasMissing = false
				}
				continue
			}

			if raw != lastRaw || fileWasMissing {
				controller.SetState(state)
				lastRaw = raw
				fileWasMissing = false
			}

		case <-controller.blinkC:
			controller.HandleBlink()
		}
	}
}

func main() {
	configureLogging()

	logger.Error("Starting Sensor Hub control program. Version: " + AppVersion)

	stateFile := defaultStateFile
	redGPIO := defaultRedGPIO
	greenGPIO := defaultGreenGPIO

	if err := os.MkdirAll(filepath.Dir(stateFile), 0777); err != nil {
		logger.Error(fmt.Sprintf("creating state directory: %v", err))
	}

	if err := os.Chmod(filepath.Dir(stateFile), 0777); err != nil {
		logger.Error(fmt.Sprintf("setting state directory permissions: %v", err))
	}

	// Create the state file if it does not already exist.
	if _, err := os.Stat(stateFile); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			file, createErr := os.OpenFile(
				stateFile,
				os.O_WRONLY|os.O_CREATE,
				0666,
			)
			if createErr != nil {
				logger.Error(fmt.Sprintf("creating state file: %v", createErr))
			} else if closeErr := file.Close(); closeErr != nil {
				logger.Error(fmt.Sprintf("closing state file: %v", closeErr))
			}
		} else {
			logger.Error(fmt.Sprintf("checking state file: %v", err))
		}
	}

	if _, err := host.Init(); err != nil {
		logger.Error(fmt.Sprintf("Initializing GPIO interface: %v", err))
	}

	redLED := NewGPIOLED("red LED", redGPIO)
	greenLED := NewGPIOLED("green LED", greenGPIO)

	controller := NewLightController(redLED, greenLED)
	defer controller.Close()

	ctx, stop := signal.NotifyContext(
		context.Background(),
		syscall.SIGTERM,
		syscall.SIGINT,
	)
	defer stop()

	logger.Info(fmt.Sprintf(
		"sensorhub controller started; state file=%s red=%s green=%s",
		stateFile,
		redGPIO,
		greenGPIO,
	))

	buttonGPIO := defaultButtonGPIO

	rawButtonPin := gpioreg.ByName(buttonGPIO)
	if rawButtonPin == nil {
		logger.Error(fmt.Sprintf("GPIO pin %s was not found", buttonGPIO))
	} else {
		buttonPin, ok := rawButtonPin.(gpio.PinIn)
		if !ok {
			logger.Error(fmt.Sprintf("GPIO pin %s is not an input-capable pin", buttonGPIO))
		} else if err := buttonPin.In(gpio.PullUp, gpio.NoEdge); err != nil {
			logger.Error(fmt.Sprintf(
				"configuring GPIO %s as a button input: %v",
				buttonGPIO,
				err,
			))
		} else {
			defer buttonPin.Halt()

			go monitorResetButton(
				ctx,
				buttonPin,
				5*time.Second,
				func() error {
					return removeSensorhubConf()
				},
			)

			logger.Info(fmt.Sprintf(
				"Reset button enabled on %s; hold for 5 seconds",
				buttonGPIO,
			))
		}
	}

	run(ctx, controller, stateFile)

	logger.Error("sensorhubcontrol shutdown requested; LEDs are off")
}
