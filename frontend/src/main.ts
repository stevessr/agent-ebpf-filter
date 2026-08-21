import { createApp } from "vue";
import type { Plugin } from "vue";
import App from "./App.vue";
import Antd from "ant-design-vue";
import router from "./router";
import "ant-design-vue/dist/reset.css";
import "./style.css";
import "./services/http";

const app = createApp(App);
app.use(Antd as unknown as Plugin);
app.use(router as unknown as Plugin);
app.mount("#app");
