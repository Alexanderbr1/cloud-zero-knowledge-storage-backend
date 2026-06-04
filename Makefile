.PHONY: up down restart logs ps build

up:
	docker compose up -d --build

down:
	docker compose down

restart:
	docker compose down && docker compose up -d --build

logs:
	docker compose logs -f --tail=200

ps:
	docker compose ps

build:
	docker compose build
