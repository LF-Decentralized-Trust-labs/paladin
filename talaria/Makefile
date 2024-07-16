clean:
	rm -rf clients

clients:
	go build
	rm -rf clients/client1
	rm -rf clients/client2
	mkdir -p clients
	mkdir -p clients/client1
	mkdir -p clients/client2
	cp baymax clients/client1
	cp baymax clients/client2