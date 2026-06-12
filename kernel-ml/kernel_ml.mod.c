#include <linux/module.h>
#include <linux/export-internal.h>
#include <linux/compiler.h>

MODULE_INFO(name, KBUILD_MODNAME);

__visible struct module __this_module
__section(".gnu.linkonce.this_module") = {
	.name = KBUILD_MODNAME,
	.init = init_module,
#ifdef CONFIG_MODULE_UNLOAD
	.exit = cleanup_module,
#endif
	.arch = MODULE_ARCH_INIT,
};

KSYMTAB_FUNC(ml_inference, "_gpl", "");
KSYMTAB_FUNC(ml_model_load, "_gpl", "");
KSYMTAB_FUNC(ml_model_free, "_gpl", "");
KSYMTAB_FUNC(extract_features, "_gpl", "");

MODULE_INFO(depends, "");


MODULE_INFO(srcversion, "4AF1BE0A8126EA3C6BE33E6");
