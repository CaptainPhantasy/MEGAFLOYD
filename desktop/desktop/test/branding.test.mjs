import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import { buildMenu } from '../src/menu.mjs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';
import os from 'node:os';
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');

test('MegaFloyd menus and fallback page preserve the stable application identity', () => {
 const configSource=fs.readFileSync(path.join(root,'src/config.mjs'),'utf8');
 const context={app:{getVersion:()=>"fixture",isPackaged:false},os,path,process:{...process,resourcesPath:"/fixture/resources"}};
 vm.runInNewContext(configSource.replace(/^import .*;$/gm, '').replaceAll('export ', '')+'\nglobalThis.name = APP_NAME;',context);
 assert.equal(context.name, 'MegaFloyd');
 let template;
 const actions = new Proxy({}, {get:()=>()=>{}});
 const Menu={buildFromTemplate(value){template=value;return value},setApplicationMenu(){}};
 buildMenu(Menu,actions);
 const labels=[];
 function walk(items){for(const item of items){if(item.label)labels.push(item.label);if(Array.isArray(item.submenu))walk(item.submenu)}}
 walk(template);
 assert.ok(labels.includes('About MegaFloyd'));
 assert.ok(labels.includes('Quit MegaFloyd'));
 assert.ok(labels.includes('Open in Terminal'));
 assert.equal(labels.some(label=>/\b(?:MEGAFLOYD|Floyd Code|SuperFloyd|Kimi)\b/.test(label)),false);
 const home=fs.readFileSync(path.join(root,'src/home.html'),'utf8');
 assert.match(home,/<title>MegaFloyd<\/title>/);
 assert.match(home,/Mega<span>Floyd<\/span>/);
 const builder=fs.readFileSync(path.join(root,'scripts/build.mjs'),'utf8');
 assert.match(builder,/const PRODUCT = 'MEGAFLOYD'/);
 assert.match(builder,/const DISPLAY_NAME = 'MegaFloyd'/);
 assert.match(builder,/'CFBundleDisplayName', 'string', DISPLAY_NAME/);
 assert.match(builder,/'CFBundleExecutable', 'string', PRODUCT/);
 assert.match(builder,/const BUNDLE_ID = 'ai\.legacy\.megafloyd'/);
});
