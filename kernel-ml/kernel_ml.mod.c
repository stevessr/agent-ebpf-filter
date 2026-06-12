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
KSYMTAB_FUNC(svm_inference, "_gpl", "");
KSYMTAB_FUNC(svm_model_load, "_gpl", "");
KSYMTAB_FUNC(lr_inference, "_gpl", "");
KSYMTAB_FUNC(lr_model_load, "_gpl", "");
KSYMTAB_FUNC(nn_inference, "_gpl", "");
KSYMTAB_FUNC(nn_model_load, "_gpl", "");
KSYMTAB_FUNC(unified_inference, "_gpl", "");
KSYMTAB_FUNC(unified_model_load, "_gpl", "");
KSYMTAB_FUNC(unified_model_free, "_gpl", "");
KSYMTAB_FUNC(dt_inference, "_gpl", "");
KSYMTAB_FUNC(knn_inference, "_gpl", "");
KSYMTAB_FUNC(nb_inference, "_gpl", "");
KSYMTAB_FUNC(gb_inference, "_gpl", "");
KSYMTAB_FUNC(ensemble_inference, "_gpl", "");
KSYMTAB_FUNC(advanced_inference, "_gpl", "");
KSYMTAB_FUNC(advanced_model_load, "_gpl", "");
KSYMTAB_FUNC(advanced_model_free, "_gpl", "");

MODULE_INFO(depends, "");


MODULE_INFO(srcversion, "0C66CFACE00E7D1FB3BD52B");
