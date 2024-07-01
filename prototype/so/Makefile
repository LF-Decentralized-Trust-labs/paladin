SUBDIRS := domains/domainA domains/domainB paladin 
all: $(SUBDIRS)
$(SUBDIRS):
	$(MAKE) -C $@

.PHONY: all $(SUBDIRS)

