import './theme/tokens.css';
import './style.css';
import { app } from './lib/api';
import { initTheme } from './theme/theme';

initTheme('auto');

const root = document.querySelector<HTMLDivElement>('#app');
if (root) {
  root.textContent = 'Goi';
  app
    .info()
    .then((info) => {
      root.textContent = `${info.name} ${info.version} (${info.os}/${info.arch})`;
    })
    .catch((err: unknown) => console.error('info', err));
}
