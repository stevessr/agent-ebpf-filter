obj-m += kernel_ml.o

kernel_ml-objs := kernel_ml_main.o ml_inference.o ml_models.o ml_advanced.o

ccflags-y := -Wall -Werror -O2

all:
	make -C /lib/modules/$(shell uname -r)/build M='$(PWD)' modules CC=clang LD=ld.lld

clean:
	make -C /lib/modules/$(shell uname -r)/build M='$(PWD)' clean

install:
	make -C /lib/modules/$(shell uname -r)/build M='$(PWD)' modules_install CC=clang LD=ld.lld
	depmod -a

load:
	insmod kernel_ml.ko

unload:
	rmmod kernel_ml

reload: unload load

.PHONY: all clean install load unload reload
