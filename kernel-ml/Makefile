obj-m += kernel_ml.o

kernel_ml-objs := kernel_ml_main.o ml_inference.o

ccflags-y := -Wall -Werror -O2

all:
	make -C /lib/modules/$(shell uname -r)/build M='$(PWD)' modules

clean:
	make -C /lib/modules/$(shell uname -r)/build M='$(PWD)' clean

install:
	make -C /lib/modules/$(shell uname -r)/build M='$(PWD)' modules_install
	depmod -a

load:
	insmod kernel_ml.ko

unload:
	rmmod kernel_ml

reload: unload load

.PHONY: all clean install load unload reload
