import {defineConfig} from 'vite';
import react from '@vitejs/plugin-react';
export default defineConfig({plugins:[react()],resolve:{alias:{'@':'D:/goagent/frontend/src'}},server:{host:'127.0.0.1',port:5175,proxy:{'/api':{target:'http://127.0.0.1:9091',changeOrigin:true,secure:false}}}});
