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

KSYMTAB_FUNC(ml_inference, "");
SYMBOL_FLAGS(ml_inference, 0x01);
KSYMTAB_FUNC(ml_model_load, "");
SYMBOL_FLAGS(ml_model_load, 0x01);
KSYMTAB_FUNC(ml_model_free, "");
SYMBOL_FLAGS(ml_model_free, 0x01);
KSYMTAB_FUNC(extract_features, "");
SYMBOL_FLAGS(extract_features, 0x01);
KSYMTAB_FUNC(svm_inference, "");
SYMBOL_FLAGS(svm_inference, 0x01);
KSYMTAB_FUNC(svm_model_load, "");
SYMBOL_FLAGS(svm_model_load, 0x01);
KSYMTAB_FUNC(lr_inference, "");
SYMBOL_FLAGS(lr_inference, 0x01);
KSYMTAB_FUNC(lr_model_load, "");
SYMBOL_FLAGS(lr_model_load, 0x01);
KSYMTAB_FUNC(nn_inference, "");
SYMBOL_FLAGS(nn_inference, 0x01);
KSYMTAB_FUNC(nn_model_load, "");
SYMBOL_FLAGS(nn_model_load, 0x01);
KSYMTAB_FUNC(unified_inference, "");
SYMBOL_FLAGS(unified_inference, 0x01);
KSYMTAB_FUNC(unified_model_load, "");
SYMBOL_FLAGS(unified_model_load, 0x01);
KSYMTAB_FUNC(unified_model_free, "");
SYMBOL_FLAGS(unified_model_free, 0x01);
KSYMTAB_FUNC(dt_inference, "");
SYMBOL_FLAGS(dt_inference, 0x01);
KSYMTAB_FUNC(knn_inference, "");
SYMBOL_FLAGS(knn_inference, 0x01);
KSYMTAB_FUNC(nb_inference, "");
SYMBOL_FLAGS(nb_inference, 0x01);
KSYMTAB_FUNC(gb_inference, "");
SYMBOL_FLAGS(gb_inference, 0x01);
KSYMTAB_FUNC(ensemble_inference, "");
SYMBOL_FLAGS(ensemble_inference, 0x01);
KSYMTAB_FUNC(advanced_inference, "");
SYMBOL_FLAGS(advanced_inference, 0x01);
KSYMTAB_FUNC(advanced_model_load, "");
SYMBOL_FLAGS(advanced_model_load, 0x01);
KSYMTAB_FUNC(advanced_model_free, "");
SYMBOL_FLAGS(advanced_model_free, 0x01);

MODULE_INFO(depends, "");


MODULE_INFO(srcversion, "3A1B1482281B1339B37B7CA");
