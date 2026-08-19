CC := gcc
CXX := g++
BASE_FLAGS := -Wall -Wextra -Werror -g -O0

.PHONY: all clean
all: bin/canary bin/canary_cpp

bin/canary: src/canary.c
	@mkdir -p bin
	$(CC) $(BASE_FLAGS) -o $@ $<

bin/canary_cpp: src/canary.cpp
	@mkdir -p bin
	$(CXX) $(BASE_FLAGS) -o $@ $<

clean:
	rm -rf bin
