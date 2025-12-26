###
# Deploy on Prod
###

buildProd:
	sudo systemctl stop flowmass.service
	cd ~/git/flowmass
	go build -o flowmass
	sudo cp -p flowmass /usr/local/bin/.
	sudo systemd-analyze verify flowmass.service
	sudo systemctl daemon-reload
	sudo systemctl restart flowmass.service
	sudo journalctl -f -u flowmass