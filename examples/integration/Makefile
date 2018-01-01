run:
	docker-compose down

	docker-compose up -d sshportal
	@echo "Waiting for sshportal to be healthy"
	@sleep 3

	docker-compose build client
	docker-compose run client /integration/_client.sh

	docker-compose down
