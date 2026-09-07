import { createServer } from 'node:http';
import { readFile } from 'node:fs/promises';
import { chromium } from 'playwright-core';

const [htmlPath, chromePath] = process.argv.slice(2);
if (!htmlPath || !chromePath) {
  throw new Error('usage: node test/dashboard_api_key_layout.mjs <dashboard-html-path> <google-chrome-path>');
}

const dashboardHTML = await readFile(htmlPath);
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
  if (url.pathname === `${resourceBase}/prices`) {
    sendJSON({ prices: {}, revision: 0 });
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

const readRects = (page, selectors) => page.evaluate((list) => {
  const read = (selector) => {
    const element = document.querySelector(selector);
    if (!element) throw new Error(`missing ${selector}`);
    const rect = element.getBoundingClientRect();
    return { left: rect.left, top: rect.top, right: rect.right, bottom: rect.bottom, width: rect.width };
  };
  const out = {};
  for (const selector of list) out[selector] = read(selector);
  return out;
}, selectors);

const intersects = (left, right) => left.left < right.right && right.left < left.right && left.top < right.bottom && right.top < left.bottom;

try {
  const desktopContext = await browser.newContext({
    locale: 'zh-CN',
    timezoneId: 'UTC',
    viewport: { width: 1300, height: 800 },
  });
  const desktopPage = await desktopContext.newPage();
  const desktopErrors = [];
  desktopPage.on('pageerror', (error) => desktopErrors.push(String(error)));

  const dataResponse = desktopPage.waitForResponse((response) => new URL(response.url()).pathname === `${resourceBase}/full-mode/data`);
  const initialResponse = desktopPage.waitForResponse((response) => new URL(response.url()).pathname === `${resourceBase}/stats/initial`);
  await desktopPage.goto(dashboardURL, { waitUntil: 'domcontentloaded' });
  await Promise.all([dataResponse, initialResponse]);
  await desktopPage.locator('#apiKeyFilterButton').waitFor({ state: 'visible' });
  await desktopPage.locator('#pricingButton').waitFor({ state: 'visible' });

  const desktopRects = await readRects(desktopPage, ['#apiKeyFilter', '#apiKeyFilterButton', '#fullModeButton', '#pricingButton']);
  const desktopPairs = [
    ['API Key filter', desktopRects['#apiKeyFilter'], 'full-mode button', desktopRects['#fullModeButton']],
    ['API Key filter button', desktopRects['#apiKeyFilterButton'], 'full-mode button', desktopRects['#fullModeButton']],
    ['API Key filter', desktopRects['#apiKeyFilter'], 'model-pricing button', desktopRects['#pricingButton']],
    ['API Key filter button', desktopRects['#apiKeyFilterButton'], 'model-pricing button', desktopRects['#pricingButton']],
  ];
  for (const [leftName, leftRect, rightName, rightRect] of desktopPairs) {
    if (intersects(leftRect, rightRect)) {
      throw new Error(`${leftName} overlaps ${rightName} at 1300px: ${JSON.stringify({ leftRect, rightRect })}`);
    }
  }
  if (desktopErrors.length > 0) {
    throw new Error(`desktop page errors: ${desktopErrors.join('\n')}`);
  }
  await desktopContext.close();

  const mobileContext = await browser.newContext({
    locale: 'zh-CN',
    timezoneId: 'UTC',
    viewport: { width: 430, height: 900 },
  });
  const mobilePage = await mobileContext.newPage();
  const mobileErrors = [];
  mobilePage.on('pageerror', (error) => mobileErrors.push(String(error)));

  const mobileDataResponse = mobilePage.waitForResponse((response) => new URL(response.url()).pathname === `${resourceBase}/full-mode/data`);
  const mobileInitialResponse = mobilePage.waitForResponse((response) => new URL(response.url()).pathname === `${resourceBase}/stats/initial`);
  await mobilePage.goto(dashboardURL, { waitUntil: 'domcontentloaded' });
  await Promise.all([mobileDataResponse, mobileInitialResponse]);
  await mobilePage.locator('#apiKeyFilterButton').waitFor({ state: 'visible' });

  const mobileRects = await readRects(mobilePage, ['.control-filters', '#rangeButton', '#apiKeyFilter', '#apiKeyFilterButton', '.control-actions']);
  const fullWidth = mobileRects['.control-filters'].width;
  if (fullWidth < 430 - 24 - 1) {
    throw new Error(`mobile control filters do not fill the shell width at 430px: ${JSON.stringify(mobileRects['.control-filters'])}`);
  }
  for (const selector of ['#rangeButton', '#apiKeyFilter', '#apiKeyFilterButton', '.control-actions']) {
    if (Math.abs(mobileRects[selector].right - mobileRects['.control-filters'].right) > 1) {
      throw new Error(`${selector} leaves empty space on the right at 430px: ${JSON.stringify({ rect: mobileRects[selector], filters: mobileRects['.control-filters'] })}`);
    }
  }
  if (mobileErrors.length > 0) {
    throw new Error(`mobile page errors: ${mobileErrors.join('\n')}`);
  }
  for (const width of [574, 700, 820, 900, 1000, 1060]) {
    await mobilePage.setViewportSize({ width, height: 800 });
    await mobilePage.waitForTimeout(100);
    const sweep = await mobilePage.evaluate(() => {
      const rect = document.getElementById('apiKeyFilterButton').getBoundingClientRect();
      return { innerWidth: window.innerWidth, buttonRight: rect.right, scrollWidth: document.documentElement.scrollWidth };
    });
    if (sweep.buttonRight > sweep.innerWidth + 1) {
      throw new Error(`API Key filter button escapes the viewport at ${width}px: ${JSON.stringify(sweep)}`);
    }
    if (sweep.scrollWidth > sweep.innerWidth + 1) {
      throw new Error(`dashboard scrolls horizontally at ${width}px: ${JSON.stringify(sweep)}`);
    }
  }
  await mobileContext.close();
} finally {
  await browser.close();
  server.close();
}
