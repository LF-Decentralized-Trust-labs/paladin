<<<<<<< HEAD
# Copyright © 2024 Kaleido, Inc.
#
# Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with
# the License. You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on
# an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the
# specific language governing permissions and limitations under the License.
#
# SPDX-License-Identifier: Apache-2.0

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

=======
all: docker
docker:
		docker build --build-arg BUILD_VERSION=${BUILD_VERSION} ${DOCKER_ARGS} -t kaleido-io/paladin .
>>>>>>> d664889020b1294dc3305d0940e7f3baff5bbce2
