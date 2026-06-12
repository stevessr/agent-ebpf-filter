obj-m += kernel_ml.o

kernel_ml-objs := kernel_ml_main.o ml_inference.o ml_models.o ml_advanced.o

ccflags-y := -Wall -Werror -O2

KVERSION ?= $(shell uname -r)
KDIR ?= /lib/modules/$(KVERSION)/build
LLVM_CC ?= clang
LLVM_LD ?= ld.lld
CUDA_HOME ?= /opt/cuda
NVCC ?= $(CUDA_HOME)/bin/nvcc
KBUILD_OUTPUTS := Module.symvers modules.order kernel_ml.ko kernel_ml.o kernel_ml.mod kernel_ml.mod.c kernel_ml.mod.o kernel_ml_main.o ml_inference.o ml_models.o ml_advanced.o .module-common.o
DKMS_ROOT ?=

all:
	@if printf "%s" "$(CURDIR)" | grep -q "[[:space:]]"; then \
		echo "kernel-ml: source path contains whitespace; staging Kbuild in /tmp"; \
		tmp=$$(mktemp -d /tmp/kernel-ml-build.XXXXXX); \
		cp -a "$(CURDIR)" "$$tmp/src"; \
		$(MAKE) -C "$$tmp/src" all KVERSION="$(KVERSION)" KDIR="$(KDIR)" LLVM_CC="$(LLVM_CC)" LLVM_LD="$(LLVM_LD)"; \
		for f in $(KBUILD_OUTPUTS); do [ ! -e "$$tmp/src/$$f" ] || cp -a "$$tmp/src/$$f" "$(CURDIR)/$$f"; done; \
		rm -rf "$$tmp"; \
	else \
		$(MAKE) -C "$(KDIR)" M="$(CURDIR)" modules CC=$(LLVM_CC) LD=$(LLVM_LD); \
	fi
	@[ ! -f Module.symvers ] || sed -i 's/[[:space:]]*$$//' Module.symvers

clean:
	@if printf "%s" "$(CURDIR)" | grep -q "[[:space:]]"; then \
		rm -f $(KBUILD_OUTPUTS) kernel_ml_cuda_helper; \
		rm -f .*.cmd *.o.cmd *.ko.cmd *.mod.cmd; \
		rm -rf .tmp_versions; \
	else \
		$(MAKE) -C "$(KDIR)" M="$(CURDIR)" clean; \
		rm -f kernel_ml_cuda_helper; \
	fi

install:
	@if printf "%s" "$(CURDIR)" | grep -q "[[:space:]]"; then \
		echo "kernel-ml: source path contains whitespace; staging Kbuild install in /tmp"; \
		tmp=$$(mktemp -d /tmp/kernel-ml-install.XXXXXX); \
		cp -a "$(CURDIR)" "$$tmp/src"; \
		$(MAKE) -C "$$tmp/src" install KVERSION="$(KVERSION)" KDIR="$(KDIR)" LLVM_CC="$(LLVM_CC)" LLVM_LD="$(LLVM_LD)"; \
		rm -rf "$$tmp"; \
	else \
		$(MAKE) -C "$(KDIR)" M="$(CURDIR)" modules_install CC=$(LLVM_CC) LD=$(LLVM_LD); \
		depmod -a; \
	fi

cuda-helper:
	@if [ ! -x "$(NVCC)" ]; then \
		echo "nvcc not found at $(NVCC); set CUDA_HOME=/path/to/cuda or NVCC=/path/to/nvcc"; \
		exit 1; \
	fi
	$(NVCC) -std=c++17 -O2 -I"$(CURDIR)" -o kernel_ml_cuda_helper cuda_infer_helper.cu

cuda-helper-self-test: cuda-helper
	./kernel_ml_cuda_helper --self-test

cuda-helper-protocol-test: cuda-helper
	python3 test_cuda_helper_protocol.py ./kernel_ml_cuda_helper

unit-test:
	python3 -m py_compile model_loader.py multi_model_exporter.py test_model_format.py test_cuda_helper_protocol.py
	python3 test_model_format.py

test: unit-test cuda-helper-protocol-test

dkms-smoke:
	@if ! command -v dkms >/dev/null 2>&1; then \
		echo "dkms not found"; \
		exit 1; \
	fi
	@root="$(DKMS_ROOT)"; \
	if [ -z "$$root" ]; then root=$$(mktemp -d /tmp/kernel-ml-dkms-smoke.XXXXXX); fi; \
	echo "kernel-ml: DKMS smoke root $$root"; \
	mkdir -p "$$root/src/kernel-ml-1.1" "$$root/dkms" "$$root/install"; \
	cp -a "$(CURDIR)/." "$$root/src/kernel-ml-1.1/"; \
	rm -f "$$root/src/kernel-ml-1.1/kernel_ml_cuda_helper"; \
	dkms add -m kernel-ml -v 1.1 --dkmstree "$$root/dkms" --sourcetree "$$root/src" --installtree "$$root/install"; \
	dkms build -m kernel-ml -v 1.1 -k "$(KVERSION)" \
		--kernelsourcedir "$(KDIR)" \
		--dkmstree "$$root/dkms" --sourcetree "$$root/src" --installtree "$$root/install"; \
	ko=$$(find "$$root/dkms" -name kernel_ml.ko | head -1); \
	test -n "$$ko"; \
	modinfo "$$ko" | grep -q "version: *1.1"; \
	modinfo "$$ko" | grep -q "backend:Inference backend"; \
	echo "kernel-ml: DKMS smoke built $$ko"

perf-profile:
	./profile_inference.sh perf $(COUNT)

nsight-profile: cuda-helper
	./profile_inference.sh nsight

load:
	insmod kernel_ml.ko

unload:
	rmmod kernel_ml

reload: unload load

.PHONY: all clean install cuda-helper cuda-helper-self-test cuda-helper-protocol-test unit-test test dkms-smoke perf-profile nsight-profile load unload reload
