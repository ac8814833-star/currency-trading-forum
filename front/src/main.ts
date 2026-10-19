import { createApp } from 'vue';
import { createPinia } from 'pinia';
import {
  ElAlert, ElButton, ElCard, ElCol, ElContainer, ElEmpty, ElForm,
  ElFormItem, ElHeader, ElInput, ElInputNumber, ElMain, ElMenu,
  ElMenuItem, ElOption, ElPagination, ElRow, ElSelect, ElTable, ElTableColumn,
} from 'element-plus';
import 'element-plus/dist/index.css';
import './styles.css';
import App from './App.vue';
import router from './router';

const app = createApp(App);
app.use(createPinia());
[
  ElAlert, ElButton, ElCard, ElCol, ElContainer, ElEmpty, ElForm,
  ElFormItem, ElHeader, ElInput, ElInputNumber, ElMain, ElMenu,
  ElMenuItem, ElOption, ElPagination, ElRow, ElSelect, ElTable, ElTableColumn,
].forEach((component) => app.component(component.name!, component));
app.use(router);
app.mount('#app');
