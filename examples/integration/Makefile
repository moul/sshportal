run:
	docker-compose down
	docker-compose up -d sshportal
	docker-compose build client
	docker-compose run client /integration/_client.sh
	docker-compose down
