import { createServer } from 'node:http';
import { readFile } from 'node:fs/promises';
import { chromium } from 'playwright-core';

const [htmlPath, chromePath] = process.argv.slice(2);
if (!htmlPath || !chromePath) {
  throw new Error('usage: node test/dashboard_pricing_host_models.mjs <dashboard-html-path> <chrome-path>');
}

const dashboardHTML = await readFile(htmlPath, 'utf8');
let savedPrices = { m: { input: 4, output: 8 } };
let revision = 1, saves = 0, hostRequests = 0, hostAuthorization = null;
let chunks = [];
const resourceBase = '/v0/resource/plugins/host-models-browser-test';

const server = createServer((request, response) => {
  const url = new URL(request.url, 'http://127.0.0.1');
  const sendJSON = (value) => {
    response.writeHead(200, { 'content-type': 'application/json' });
    response.end(JSON.stringify(value));
  };

  if (url.pathname === `${resourceBase}/full-mode/prices/save`) {
    if (url.searchParams.get('stage') === 'begin') { chunks = []; sendJSON({ upload: 'save-upload' }); return; }
    if (url.searchParams.get('stage') === 'chunk') { chunks[Number(url.searchParams.get('index'))] = request.headers['x-full-mode-payload']; sendJSON({}); return; }
    if (url.searchParams.get('stage') === 'commit') {
      savedPrices = JSON.parse(Buffer.from(chunks.join(''), 'base64url').toString('utf8')).prices;
      revision++; saves++; sendJSON({ prices: savedPrices, revision }); return;
    }
  }
  if (url.pathname === `${resourceBase}/full-mode/prices/sync`) {
    if (url.searchParams.get('stage') === 'begin') { chunks = []; sendJSON({ upload: 'sync-upload' }); return; }
    if (url.searchParams.get('stage') === 'chunk') { chunks[Number(url.searchParams.get('index'))] = request.headers['x-full-mode-payload']; sendJSON({}); return; }
    if (url.searchParams.get('stage') === 'commit') {
      revision++; sendJSON({ prices: savedPrices, revision }); return;
    }
  }
  if (url.pathname === `${resourceBase}/full-mode/models`) {
    sendJSON({ models: Object.keys(savedPrices).sort(), observed: 1, priced: Object.keys(savedPrices).length, generated_at: '2026-11-25T00:00:00Z' });
    return;
  }
  // Host client API used by the optional import: it is the only route that may
  // see an Authorization header from the price dialog.
  if (url.pathname === '/v1/models') {
    hostRequests++;
    hostAuthorization = request.headers['authorization'] || null;
    sendJSON({ object: 'list', data: [{ id: 'host-model-a' }, { id: 'host-model-b' }] });
    return;
  }
  if (url.pathname === `${resourceBase}/dashboard`) {
    response.writeHead(200, { 'content-type': 'text/html; charset=utf-8' });
    response.end(dashboardHTML);
    return;
  }
  if (url.pathname === `${resourceBase}/full-mode/data`) { sendJSON({ api_key_tracking_enabled: false, api_key_uses_default_secret: false, api_key_labels: {} }); return; }
  if (url.pathname === `${resourceBase}/preferences`) { sendJSON({}); return; }
  if (url.pathname === `${resourceBase}/stats/initial`) { sendJSON({ generated_at: '2026-11-25T00:00:00.000Z', last_used: '2026-11-25T00:00:00.000Z', models: [], sources: [], api_keys: [], bucket_seconds: 86400 }); return; }
  if (url.pathname === `${resourceBase}/stats/trends`) { sendJSON({ model_series: [], bucket_seconds: 86400 }); return; }
  if (url.pathname === `${resourceBase}/stats/groups` || url.pathname === `${resourceBase}/requests`) { sendJSON({ items: [], total: 0 }); return; }
  if (url.pathname === `${resourceBase}/costs`) { sendJSON({ summary: { requests: 0, priced_requests: 0, unpriced_requests: 0 }, models: [], price_book_revision: 0 }); return; }
  if (url.pathname === `${resourceBase}/prices` || url.pathname === `${resourceBase}/full-mode/prices`) { sendJSON({ prices: savedPrices, revision }); return; }
  sendJSON({});
});

await new Promise((resolve, reject) => {
  server.once('error', reject);
  server.listen(0, '127.0.0.1', resolve);
});

const dashboardURL = `http://127.0.0.1:${server.address().port}${resourceBase}/dashboard#session=host-models-test`;
const browser = await chromium.launch({ executablePath: chromePath, headless: true });

try {
  const context = await browser.newContext({ locale: 'en-US', viewport: { width: 1300, height: 900 } });
  const page = await context.newPage();
  const errors = [];
  page.on('pageerror', (error) => errors.push(String(error)));

  await page.goto(dashboardURL);
  await page.locator('#pricingButton').click();
  await page.waitForFunction(() => /Model list: \d+/.test(document.getElementById('modelCatalogStatus').textContent));

  if (await page.locator('#cliModelsKeyInput').count() !== 0) throw Error('Price dialog must not gate on a CLIProxyAPI API Key');
  if (await page.locator('.price-row').count() !== 1) throw Error('Automatic model list must render the priced model only');

  // Without a key the import is refused locally and never reaches the host.
  await page.locator('#loadHostModels').click();
  await page.waitForFunction(() => document.getElementById('priceError').textContent.length > 0);
  if (hostRequests !== 0) throw Error('Host import must not call /v1/models without an API key');

  // With a key the host list is merged into the editor.
  await page.locator('#hostModelsKeyInput').fill('host-test-key');
  const hostResponse = page.waitForResponse((r) => new URL(r.url()).pathname === '/v1/models');
  await page.locator('#loadHostModels').click();
  await hostResponse;
  await page.waitForFunction(() => document.querySelectorAll('.price-row').length === 3);
  if (hostRequests !== 1 || hostAuthorization !== 'Bearer host-test-key') throw Error(`Host import credentials: ${hostRequests} / ${hostAuthorization}`);
  await page.waitForFunction(() => /Host model list: 2/.test(document.getElementById('hostModelsStatus').textContent));

  // Only models that actually carry a price are persisted.
  await page.locator('.price-row', { hasText: 'host-model-a' }).locator('.price-input').fill('2');
  await page.locator('#savePricing').click();
  await page.locator('#pricingDialog').waitFor({ state: 'hidden' });
  if (saves !== 1) throw Error('Price save did not happen');
  const savedKeys = Object.keys(savedPrices).sort().join(',');
  if (savedKeys !== 'host-model-a,m') throw Error(`Saved models: ${savedKeys}`);

  // Synchronization still uses the session only.
  await page.locator('#pricingButton').click();
  const syncResponse = page.waitForResponse((r) => r.url().includes('/full-mode/prices/sync?stage=commit'));
  await page.locator('#syncPrices').click();
  await syncResponse;

  // The API key is page-scoped: a reload drops it, the saved price stays.
  await page.goto('about:blank');
  await page.goto(dashboardURL);
  await page.locator('#pricingButton').click();
  await page.waitForFunction(() => document.querySelectorAll('.price-row').length === 2);
  if (await page.locator('#hostModelsKeyInput').inputValue() !== '') throw Error('API key must not survive a reload');

  if (errors.length) throw Error(errors.join('\n'));
  console.log('Host model import, credential scoping and price save passed');
} finally {
  await browser.close();
  await new Promise((resolve) => server.close(resolve));
}
