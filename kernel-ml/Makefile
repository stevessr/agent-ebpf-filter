obj-m += kernel_ml.o

kernel_ml-objs := kernel_ml_main.o ml_inference.o ml_models.o ml_advanced.o

ccflags-y := -Wall -Werror -O2

KVERSION ?= $(shell uname -r)
KDIR ?= /lib/modules/$(KVERSION)/build
LLVM_CC ?= clang
LLVM_LD ?= ld.lld
CUDA_HOME ?= /opt/cuda
NVCC ?= $(CUDA_HOME)/bin/nvcc

all:
	$(MAKE) -C '$(KDIR)' M='$(CURDIR)' modules CC=$(LLVM_CC) LD=$(LLVM_LD)

clean:
	$(MAKE) -C '$(KDIR)' M='$(CURDIR)' clean
	rm -f kernel_ml_cuda_helper

install:
	$(MAKE) -C '$(KDIR)' M='$(CURDIR)' modules_install CC=$(LLVM_CC) LD=$(LLVM_LD)
	depmod -a

cuda-helper:
	@if [ ! -x '$(NVCC)' ]; then \
		echo "nvcc not found at $(NVCC); set CUDA_HOME=/path/to/cuda or NVCC=/path/to/nvcc"; \
		exit 1; \
	fi
	$(NVCC) -std=c++17 -O2 -I'$(CURDIR)' -o kernel_ml_cuda_helper cuda_infer_helper.cu

cuda-helper-self-test: cuda-helper
	./kernel_ml_cuda_helper --self-test

load:
	insmod kernel_ml.ko

unload:
	rmmod kernel_ml

reload: unload load

.PHONY: all clean install cuda-helper cuda-helper-self-test load unload reload
