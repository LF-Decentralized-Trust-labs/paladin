all: docker

# include the common tasks
CREATE_TEST_DB := 0
include ./Makefile.common

docker:
ifeq ($(CREATE_TEST_DB),1)
	$(MAKE) start-db
	docker build --network=host --build-arg POSTGRES_HOST=host.docker.internal --build-arg BUILD_VERSION=${BUILD_VERSION} ${DOCKER_ARGS} -t kaleido-io/paladin .
	$(MAKE) stop-db
else
	docker build --network=host --build-arg POSTGRES_HOST=host.docker.internal --build-arg BUILD_VERSION=${BUILD_VERSION} ${DOCKER_ARGS} -t kaleido-io/paladin .
endif

