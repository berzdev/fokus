DOCKER_USERNAME ?= pb
APPLICATION_NAME ?= fokus
hellomake:
		echo "1234"
dev:
		docker build --tag ${DOCKER_USERNAME}/${APPLICATION_NAME}:dev .
build:
		docker build --tag ${DOCKER_USERNAME}/${APPLICATION_NAME}:${GIT_HASH} .
push:
		docker push ${DOCKER_USERNAME}/${APPLICATION_NAME}:${GIT_HASH}
restart:
		docker compose -f docker-compose.yml down
		docker compose -f docker-compose.yml up -d
logs:
		docker compose -f docker-compose.yml logs
ps:
		docker compose -f docker-compose.yml ps
#https://earthly.dev/blog/docker-and-makefiles/
