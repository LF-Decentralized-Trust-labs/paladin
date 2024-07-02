SUBDIRS := domains/domainA paladin 

default: build_all test_e2e

build_all: $(SUBDIRS)
	for dir in $(SUBDIRS); do \
		$(MAKE) -C $$dir build; \
	done

clean:
	for dir in $(SUBDIRS); do \
		$(MAKE) -C $$dir clean; \
	done

.PHONY: all $(SUBDIRS) clean

