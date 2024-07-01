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
SOURCES=domainB.cc LoadDomain.pb.cc

# Object files
OBJECTS=$(SOURCES:.cc=.o)

# Shared library name
LIBNAME=libdomainb.so

# Build target
all: $(LIBNAME)

$(LIBNAME): $(OBJECTS)
	$(CXX) -shared -o $@ $(OBJECTS) $(LDFLAGS)

%.o: %.cc
	$(CXX) $(CXXFLAGS) -c $< -o $@

clean:
	rm -f $(OBJECTS) $(LIBNAME)

.PHONY: all clean
