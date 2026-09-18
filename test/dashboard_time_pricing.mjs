import { createServer } from 'node:http';
import { readFile } from 'node:fs/promises';
import { chromium } from 'playwright-core';

const [htmlPath, chromePath] = process.argv.slice(2);
if (!htmlPath || !chromePath) {
  throw new Error('usage: node test/dashboard_api_key_layout.mjs <dashboard-html-path> <google-chrome-path>');
}

const dashboardHTML = (await readFile(htmlPath,'utf8')).replace('function collectPricing(){', 'window.testCollect=()=>collectPricing();window.testReload=book=>{prices=book;renderPricingEditor();};function collectPricing(){');
const resourceBase = '/v0/resource/plugins/api-key-layout-browser-test';
const server = createServer((request, response) => {
  const url = new URL(request.url, 'http://127.0.0.1');
  const sendJSON = (value) => {
    response.writeHead(200, { 'content-type': 'application/json' });
    response.end(JSON.stringify(value));
  };

  if (url.pathname === `${resourceBase}/dashboard`) {
    response.writeHead(200, { 'content-type': 'text/html; charset=utf-8' });
    response.end(dashboardHTML);
    return;
  }
  if (url.pathname === `${resourceBase}/full-mode/data`) {
    sendJSON({
      api_key_tracking_enabled: true,
      api_key_uses_default_secret: false,
      api_key_labels: {},
    });
    return;
  }
  if (url.pathname === `${resourceBase}/preferences`) {
    sendJSON({});
    return;
  }
  if (url.pathname === `${resourceBase}/stats/initial`) {
    sendJSON({
      generated_at: '2026-09-07T00:00:00.000Z',
      last_used: '2026-09-07T00:00:00.000Z',
      models: [],
      sources: [],
      api_keys: [
        { ref: 'g1:0123456789abcdef0123456789abcdef', key: 'sk-test-key', status: 'available' },
      ],
      bucket_seconds: 86400,
    });
    return;
  }
  if (url.pathname === `${resourceBase}/stats/trends`) {
    sendJSON({ model_series: [], bucket_seconds: 86400 });
    return;
  }
  if (url.pathname === `${resourceBase}/stats/groups` || url.pathname === `${resourceBase}/requests`) {
    sendJSON({ items: [], total: 0 });
    return;
  }
  if (url.pathname === `${resourceBase}/costs`) {
    sendJSON({ summary: { requests: 0, priced_requests: 0, unpriced_requests: 0 }, models: [], price_book_revision: 0 });
    return;
  }
  if (url.pathname === `${resourceBase}/prices` || url.pathname === `${resourceBase}/full-mode/prices`) {
    sendJSON({ prices: {m:{input:4,output:8}}, revision: 1 });
    return;
  }
  sendJSON({});
});

await new Promise((resolve, reject) => {
  server.once('error', reject);
  server.listen(0, '127.0.0.1', resolve);
});

const address = server.address();
const dashboardURL = `http://127.0.0.1:${address.port}${resourceBase}/dashboard#session=layout-test`;
const browser = await chromium.launch({ executablePath: chromePath, headless: true });

try {
 const context=await browser.newContext({locale:'en-US',viewport:{width:1300,height:900}});
 const page=await context.newPage();const errors=[];page.on('pageerror',e=>errors.push(String(e)));
 await page.goto(dashboardURL);await page.locator('#pricingButton').click();
 const panel=page.locator('.time-pricing');await panel.locator('summary').click();
 await panel.locator('.time-zone').fill('Asia/Shanghai');await panel.locator('.add-time-tier').click();
 const row=panel.locator('.time-tier-row');await row.locator('.time-name').fill('night');await row.locator('.time-days').fill('1,7');await row.locator('.time-input').fill('1');
 const collected=await page.evaluate(()=>window.testCollect());
 if(collected.m.time_zone!=='Asia/Shanghai'||collected.m.time_tiers[0].input!==1||collected.m.time_tiers[0].days.join(',')!=='1,7')throw Error(JSON.stringify(collected));
 await page.evaluate(book=>window.testReload(book),collected);
 await panel.locator('summary').click();if(await panel.locator('.time-name').inputValue()!=='night')throw Error('round trip lost schedule');
 await panel.locator('.add-time-tier').click();await panel.locator('.time-name').nth(1).fill('overlap');
 const error=await page.evaluate(()=>{try{window.testCollect();return '';}catch(e){return e.message;}});if(!error)throw Error('overlap accepted');
 await panel.locator('.time-tier-row').nth(1).locator('button').click();
 if(errors.length)throw Error(errors.join('\n'));
 await page.screenshot({path:htmlPath+'.png',fullPage:true});
 console.log('Time pricing browser round trip, overlap validation and deletion passed');
} finally {await browser.close();await new Promise(resolve=>server.close(resolve));}
