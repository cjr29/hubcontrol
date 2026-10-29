package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"periph.io/x/conn/v3/gpio"
)

func monitorResetButton(
	ctx context.Context,
	button gpio.PinIn,
	holdDuration time.Duration,
	reset func() error,
) {
	const pollInterval = 25 * time.Millisecond

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	var pressedAt time.Time
	triggered := false

	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C:
			pressed := button.Read() == gpio.Low

			if !pressed {
				// Button was released. Make the next press eligible.
				pressedAt = time.Time{}
				triggered = false
				continue
			}

			if pressedAt.IsZero() {
				pressedAt = time.Now()
				logger.Debug("reset button pressed")
				continue
			}

			if !triggered && time.Since(pressedAt) >= holdDuration {
				triggered = true
				logger.Debug("reset button held for 5 seconds; clearing Wi-Fi configuration")
				logger.Debug("Set LED to slow flashing GREEN...")
				if err := writeStateFile(GREENSLOW); err != nil {
					logger.Error(fmt.Sprintf("Failed to set LED state: %v", err))
				}
				if err := reset(); err != nil {
					logger.Error(fmt.Sprintf("Failed to remove sensorhub.conf: %v", err))
				}
			}
		}
	}
}

func removeSensorhubConf() error {
	err := os.Remove(sensorHubConfigFile)
	if err == nil || errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
