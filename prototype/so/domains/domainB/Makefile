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

# Compiler
CXX=g++

# Include directory for protobuf
PROTOBUF_INCLUDE_PATH=/opt/homebrew/include

# Abseil library path
ABSL_LIB_PATH=/opt/homebrew/lib
#
# Compiler flags
CXXFLAGS=-std=c++17 -fPIC -Wall -O2 -I$(PROTOBUF_INCLUDE_PATH)

# Linker flags
LDFLAGS=-L$(ABSL_LIB_PATH) -labsl_strings -labsl_synchronization -labsl_time -labsl_log_internal_globals -labsl_log_internal_message -labsl_log_internal_check_op -lprotobuf

# Source files
SOURCES=Domain.pb.cc domainB.cc 

# Object files
OBJECTS=$(SOURCES:.cc=.o)

# Shared library name
LIBNAME=libdomainb.so

# Build target
all: $(LIBNAME)

Domain.pb.cc: Domain.proto
	protoc --cpp_out=. Domain.proto

Domain.proto:
	cp ../../paladin/pkg/protobuf/Domain.proto .

$(LIBNAME): $(OBJECTS)
	$(CXX) -shared -o $@ $(OBJECTS) $(LDFLAGS)

%.o: %.cc
	$(CXX) $(CXXFLAGS) -c $< -o $@

clean:
	rm -f $(OBJECTS) $(LIBNAME) Domain.pb.cc Domain.pb.h Domain.proto

log:
	@echo "CXX: $(CXX)"
	@echo "CXXFLAGS: $(CXXFLAGS)"
	@echo "LDFLAGS: $(LDFLAGS)"
	@echo "SOURCES: $(SOURCES)"
	@echo "OBJECTS: $(OBJECTS)"
	@echo "LIBNAME: $(LIBNAME)"
.PHONY: all clean
