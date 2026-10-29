# hubcontrol
Controls LED activity light and monitors reest pushbutton for Sensor Hub

Install the binary:
sudo install -m 0755 sensorhub /usr/local/bin/hubcontrol
sudo mkdir -p /var/sensorhub
echo OFF | sudo tee /var/sensorhub/led.state

Enable and Start service:
sudo systemctl daemon-reload
sudo systemctl enable --now sensorhubcontrol.service

Check status:
sudo systemctl status sensorhubcontrol.service
sudo journalctl -u sensorhubcontrol.service -f

Change the LED state atomically from another process:
printf 'REDON\n' > /var/sensorhub/led.state

For production use, write through a temporary file and rename it so the controller never observes a partially written value:
printf 'GREENFAST\n' > /var/sensorhub/led.state.tmp
mv /var/sensorhub/led.state.tmp /var/sensorhub/led.state

Systemd shutdown or:
sudo systemctl stop sensorhubcontrol.service