import { createServer } from 'node:http';
import { readFile } from 'node:fs/promises';
import { chromium } from 'playwright-core';

const [htmlPath, chromePath, scenario = 'exclusive'] = process.argv.slice(2);
if (!htmlPath || !chromePath) {
  throw new Error('usage: node test/dashboard_date_range.mjs <dashboard-html-path> <google-chrome-path> [exclusive|end-time|end-time-reset|quick-preset|reverse|los-angeles-dst|token-unit|recent-open|expired-open|trend-total|trend-total-full]');
}
if (!['exclusive', 'end-time', 'end-time-reset', 'quick-preset', 'reverse', 'los-angeles-dst', 'token-unit', 'recent-open', 'expired-open', 'trend-total', 'trend-total-full'].includes(scenario)) {
  throw new Error(`unknown dashboard date-range browser scenario: ${scenario}`);
}

let dashboardHTML = await readFile(htmlPath, 'utf8');
if (scenario.startsWith('trend-total')) {
  dashboardHTML = dashboardHTML.replace('function trendNumber(value)', "window.__trendTest={pointStackTotal:pointStackTotal,trendGeometry:trendGeometry,trendBusinessTotal:trendBusinessTotal,trendCacheReadTokens:trendCacheReadTokens,exportPNG:typeof exportPNG==='function'?exportPNG:null,disableDownload:function(){downloadBlob=function(){};}};function trendNumber(value)");
}
const resourceBase = '/v0/resource/plugins/calendar-browser-test';
const timezoneId = scenario === 'los-angeles-dst' ? 'America/Los_Angeles' : 'UTC';
const initialRange = scenario === 'los-angeles-dst'
  ? { start: '2026-08-23T07:00:00.000Z', end: '2026-08-24T07:00:00.000Z' }
  : ['recent-open', 'expired-open'].includes(scenario)
  ? { start: '2026-08-20T01:02:03.000Z', end: '2026-08-21T04:05:06.000Z' }
  : { start: '2026-08-23T00:00:00.000Z', end: '2026-08-24T00:00:00.000Z' };
const emptyInitial = {
  generated_at: '2026-08-23T00:00:00.000Z',
  last_used: '0001-01-01T00:00:00.000Z',
  models: [],
  sources: [],
  bucket_seconds: 86400,
};
const tokenUnitInitial = {
  generated_at: '2026-08-23T00:00:00.000Z',
  last_used: '2026-08-23T12:00:00.000Z',
  models: [{ model: 'browser-test', requests: 1, input_tokens: 1000000000, output_tokens: 230000000, reasoning_tokens: 0, cache_read_tokens: 0, cache_creation_tokens: 0, total_tokens: 1230000000 }],
  sources: [],
  bucket_seconds: 86400,
};
const trendTotalInitial = {
  generated_at: '2026-08-23T00:00:00.000Z',
  last_used: '2026-08-23T12:00:00.000Z',
  models: [],
  series: [{
    hour: '2026-08-23T12:00:00Z', requests: 1,
    input_tokens: 100, output_tokens: 20, reasoning_tokens: 5,
    cache_read_tokens: 80, cache_creation_tokens: 0, total_tokens: 125,
  }],
  sources: [],
  bucket_seconds: 86400,
};
const initialPayload = scenario === 'token-unit' ? tokenUnitInitial : scenario.startsWith('trend-total') ? trendTotalInitial : emptyInitial;
const savedTokenDisplayModes = [];
const initialStatsURLs = [];
let persistedPreferences = {
  time_range_mode: 'custom', time_range_start: initialRange.start, time_range_end: initialRange.end,
  request_page_size: 25, dimension_page_size: 50,
  hidden_request_columns: ['model'], hidden_dimension_columns: ['provider'],
  token_display_mode: scenario === 'token-unit' ? 'B' : 'full',
  last_dashboard_open_at: scenario === 'recent-open' ? '2026-08-23T11:50:00.000Z'
    : scenario === 'expired-open' ? '2026-08-23T11:44:00.000Z' : '',
};

async function setTimePickerValue(page, boundary, values) {
  for (const [part, value] of Object.entries(values)) {
    await page.locator(`#${boundary}TimePicker [data-time-part="${part}"]`).evaluate((field, nextValue) => {
      field.value = nextValue;
      field.dispatchEvent(new Event('input', { bubbles: true }));
    }, value);
  }
}

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
  if (url.pathname === `${resourceBase}/preferences`) {
    if (url.searchParams.get('save') === '1') {
      savedTokenDisplayModes.push(url.searchParams.get('token_display_mode'));
      persistedPreferences = {
        request_page_size: url.searchParams.get('request_page_size'),
        dimension_page_size: url.searchParams.get('dimension_page_size'),
        time_range_mode: url.searchParams.get('time_range_mode'),
        token_display_mode: url.searchParams.get('token_display_mode'),
        last_dashboard_open_at: url.searchParams.get('last_dashboard_open_at'),
        time_range_start: url.searchParams.get('time_range_start'),
        time_range_end: url.searchParams.get('time_range_end'),
        hidden_request_columns: url.searchParams.getAll('hidden_request_column'),
        hidden_dimension_columns: url.searchParams.getAll('hidden_dimension_column'),
      };
      sendJSON(persistedPreferences);
      return;
    }
    sendJSON(persistedPreferences);
    return;
  }
  if (url.pathname === `${resourceBase}/full-mode/data`) {
    sendJSON({ api_key_tracking_enabled: false, api_key_labels: {} });
    return;
  }
  if (url.pathname === `${resourceBase}/stats/initial`) {
    initialStatsURLs.push(url.toString());
    sendJSON(initialPayload);
    return;
  }
  if (url.pathname === `${resourceBase}/stats/trends`) {
    sendJSON(scenario.startsWith('trend-total') ? {
      model_series: [{
        hour: '2026-08-23T12:00:00Z', model: 'browser-test', requests: 1,
        input_tokens: 100, output_tokens: 20, reasoning_tokens: 5,
        cache_read_tokens: 80, cache_creation_tokens: 0, total_tokens: 125,
      }], bucket_seconds: 86400,
    } : { model_series: [], bucket_seconds: 86400 });
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
const dashboardURL = `http://127.0.0.1:${address.port}${resourceBase}/dashboard`;
const browser = await chromium.launch({ executablePath: chromePath, headless: true });

try {
  const context = await browser.newContext({ locale: 'zh-CN', timezoneId });
  const page = await context.newPage();
  const pageErrors = [];
  page.on('pageerror', (error) => pageErrors.push(error));

  await page.clock.install({ time: new Date('2026-08-23T12:00:00.000Z') });
  await page.goto(dashboardURL + (scenario === 'trend-total-full' ? '#session=browser-test-session' : ''), { waitUntil: 'domcontentloaded' });
  await page.waitForResponse((response) => new URL(response.url()).pathname === `${resourceBase}/stats/initial`);

  if (scenario === 'token-unit') {
    const tokenButton = page.locator('#tokenUnitButton');
    const totalTokens = page.locator('#totalTokens');
    const expected = [
      ['B', '1.23B', 'full'],
      ['完整', '1,230,000,000', 'k'],
      ['k', '1,230,000k', 'm'],
      ['m', '1,230m', 'B'],
    ];
    for (const [buttonText, totalText, savedMode] of expected) {
      if (await tokenButton.textContent() !== buttonText) {
        throw new Error(`expected token unit button ${buttonText}, got ${await tokenButton.textContent()}`);
      }
      if (await totalTokens.textContent() !== totalText) {
        throw new Error(`expected total tokens ${totalText}, got ${await totalTokens.textContent()}`);
      }
      await Promise.all([
        page.waitForResponse((response) => {
          const savedURL = new URL(response.url());
          return savedURL.pathname === `${resourceBase}/preferences`
            && savedURL.searchParams.get('save') === '1'
            && savedURL.searchParams.get('token_display_mode') === savedMode;
        }),
        tokenButton.click(),
      ]);
    }
    const toggledModes = savedTokenDisplayModes.slice(-4);
    if (JSON.stringify(toggledModes) !== JSON.stringify(['full', 'k', 'm', 'B'])) {
      throw new Error(`expected saved token display modes full,k,m,B, got ${savedTokenDisplayModes.join(',')}`);
    }
    if (await tokenButton.textContent() !== 'B' || await totalTokens.textContent() !== '1.23B') {
      throw new Error(`expected token unit cycle to return to B, got ${await tokenButton.textContent()} / ${await totalTokens.textContent()}`);
    }
  }

  if (scenario.startsWith('trend-total')) {
    await page.locator('#chart rect.bar-input').waitFor();
    const populatedHitIndex = await page.locator('#chart .bar-hit').evaluateAll((nodes) => nodes.findIndex((node) => !node.getAttribute('aria-label').includes('该时间段内没有请求')));
    const matchingHit = populatedHitIndex >= 0 ? page.locator('#chart .bar-hit').nth(populatedHitIndex) : null;
    if (!matchingHit) throw new Error('could not locate hit target for populated trend bar');
    await page.locator('#chart rect.bar-input').waitFor();
    const rects = page.locator('#chart rect.bar-input, #chart rect.bar-cache-read, #chart rect.bar-output');
    if (await rects.count() !== 3) throw new Error(`expected input/cache/output SVG bars, got ${await rects.count()}`);
    const geometry = await rects.evaluateAll((nodes) => nodes.map((node) => ({
      className: node.getAttribute('class'), y: Number(node.getAttribute('y')), height: Number(node.getAttribute('height')),
    })));
    const input = geometry.find((item) => item.className === 'bar-input');
    const cache = geometry.find((item) => item.className === 'bar-cache-read');
    const output = geometry.find((item) => item.className === 'bar-output');
    if (!input || !cache || !output || !(cache.y === input.y && cache.y + cache.height <= input.y + input.height)
      || !(output.y < input.y && output.y + output.height <= input.y)) {
      throw new Error(`cache must overlay input and output must sit above it: ${JSON.stringify(geometry)}`);
    }
    const axisLabels = await page.locator('#chart text.axis-label').allTextContents();
    if (!axisLabels.includes('120')) throw new Error(`expected geometric Y-axis maximum 120, got ${axisLabels.join(',')}`);
    await matchingHit.hover();
    const tooltipText = await page.locator('#tooltip').textContent();
    if (!tooltipText.includes('125')) throw new Error(`expected backend total 125 in tooltip, got ${tooltipText}`);

    const edgeResults = await page.evaluate(() => {
      const { pointStackTotal, trendGeometry, trendBusinessTotal, trendCacheReadTokens } = window.__trendTest;
      const point = { input: 100, output: 20, cacheRead: 80, total: 125 };
      return {
        original: pointStackTotal({ input: 48342180, output: 168768, cacheRead: 46221824 }),
        overCache: trendGeometry({ input: 100, output: 20, cacheRead: 150 }, true, true, true),
        zeroInput: trendGeometry({ input: 0, output: 20, cacheRead: 80 }, true, true, true),
        onlyCache: trendGeometry(point, false, false, true),
        allHidden: trendGeometry(point, false, false, false),
        zeroTotal: trendBusinessTotal({ input: 100, output: 20, total: 0 }),
        missingTotal: trendBusinessTotal({ input: 100, output: 20 }),
        legacyCache: trendCacheReadTokens({ cache_read_tokens: 0, cached_tokens: 40 }),
      };
    });
    if (edgeResults.original !== 48510948 || edgeResults.overCache.total !== 120 || edgeResults.overCache.cacheHeight !== 100
      || edgeResults.zeroInput.total !== 20 || edgeResults.zeroInput.cacheHeight !== 0 || edgeResults.onlyCache.total !== 80
      || edgeResults.allHidden.total !== 0 || edgeResults.zeroTotal !== 0 || edgeResults.missingTotal !== 120 || edgeResults.legacyCache !== 40) {
      throw new Error('trend boundary calculations failed: ' + JSON.stringify(edgeResults));
    }

    const verifyPNG = async (expectedInputHeight, expectedCacheHeight, expectedOutputHeight, expectedMax) => {
      if (scenario !== 'trend-total-full') return;
      const draws = await page.evaluate(async () => {
        const records = [], original = CanvasRenderingContext2D.prototype.fillRect;
        const style = getComputedStyle(document.documentElement);
        const colors = ['--input-color', '--cache-read-color', '--output-color'].map((key) => style.getPropertyValue(key).trim());
        CanvasRenderingContext2D.prototype.fillRect = function(x, y, width, height) {
          if (width <= 34 && height > 0 && colors.includes(this.fillStyle)) records.push({ color: this.fillStyle, y, height });
          return original.call(this, x, y, width, height);
        };
        window.__trendTest.disableDownload();
        try { await window.__trendTest.exportPNG(); } finally {
          CanvasRenderingContext2D.prototype.fillRect = original;
          // Canvas encoding callback can execute later; keep the test download stub installed.
        }
        return { records, colors };
      });
      const [inputColor, cacheColor, outputColor] = draws.colors;
      const near = (a, b) => Math.abs(a - b) < 1e-7;
      const check = (color, tokens, y) => {
        const rect = draws.records.find((record) => record.color === color);
        if (tokens === 0) { if (rect) throw new Error('unexpected hidden PNG bar: ' + JSON.stringify(rect)); return; }
        if (!rect || !near(rect.height, tokens / expectedMax * 452) || !near(rect.y, y)) {
          throw new Error('PNG geometry mismatch: ' + JSON.stringify({ rect, tokens, expectedMax, y }));
        }
      };
      const baseHeight = expectedInputHeight || expectedCacheHeight;
      check(inputColor, expectedInputHeight, 834 - baseHeight / expectedMax * 452);
      check(cacheColor, expectedCacheHeight, 834 - baseHeight / expectedMax * 452);
      check(outputColor, expectedOutputHeight, 834 - (baseHeight + expectedOutputHeight) / expectedMax * 452);
    };
    await verifyPNG(100, 80, 20, 120);

    await page.locator('.series-key-button[data-series="cacheRead"]').click();
    if (await page.locator('#chart rect.bar-cache-read').count() !== 0) throw new Error('hidden cache series must remove cache overlay');
    if (!(await page.locator('#chart text.axis-label').allTextContents()).includes('120')) throw new Error('hiding cache must not change geometric maximum');
    await verifyPNG(100, 0, 20, 120);
    await page.locator('.series-key-button[data-series="input"]').click();
    const hiddenInputGeometry = await page.locator('#chart rect.bar-output').evaluate((node) => ({ y: Number(node.getAttribute('y')), height: Number(node.getAttribute('height')) }));
    if (await page.locator('#chart rect.bar-cache-read').count() !== 0 || hiddenInputGeometry.height <= 0) throw new Error('hidden input/cache boundary rendered incorrectly');
    await page.locator('.series-key-button[data-series="cacheRead"]').click();
    const cacheOnly = await page.locator('#chart rect.bar-cache-read').evaluate((node) => ({ y: Number(node.getAttribute('y')), height: Number(node.getAttribute('height')) }));
    const outputWithCache = await page.locator('#chart rect.bar-output').evaluate((node) => ({ y: Number(node.getAttribute('y')), height: Number(node.getAttribute('height')) }));
    if (!(cacheOnly.height > 0 && outputWithCache.y < cacheOnly.y)) throw new Error(`cache-only geometry must be below output: ${JSON.stringify({ cacheOnly, outputWithCache })}`);
  }

  if (scenario === 'recent-open' || scenario === 'expired-open') {
    const initialURL = new URL(initialStatsURLs[0]);
    const expectedStart = scenario === 'expired-open'
      ? '2026-08-23T00:00:00.000Z'
      : initialRange.start;
    const expectedEnd = scenario === 'expired-open'
      ? '2026-08-24T00:00:00.000Z'
      : initialRange.end;
    if (initialURL.searchParams.get('start') !== expectedStart || initialURL.searchParams.get('end') !== expectedEnd) {
      throw new Error(`expected ${scenario} initial range ${expectedStart}..${expectedEnd}, got ${initialURL.searchParams.get('start')}..${initialURL.searchParams.get('end')}`);
    }
    if (persistedPreferences.time_range_start !== expectedStart || persistedPreferences.time_range_end !== expectedEnd
      || !persistedPreferences.last_dashboard_open_at.startsWith('2026-08-23T12:00:00.')
      || persistedPreferences.request_page_size !== '25' || persistedPreferences.dimension_page_size !== '50'
      || persistedPreferences.hidden_request_columns.join(',') !== 'model'
      || persistedPreferences.hidden_dimension_columns.join(',') !== 'provider') {
      throw new Error(`incorrect persisted open state: ${JSON.stringify(persistedPreferences)}`);
    }
    const reloaded = page.waitForResponse(response => new URL(response.url()).pathname === `${resourceBase}/stats/initial`);
    await page.reload();
    const reloadedURL = new URL((await reloaded).url());
    if (reloadedURL.searchParams.get('start') !== expectedStart || reloadedURL.searchParams.get('end') !== expectedEnd) {
      throw new Error(`saved range lost on reload: ${reloadedURL}`);
    }
  }

  if (!['recent-open', 'expired-open'].includes(scenario)) {
  await page.locator('#rangeButton').click();
  if (scenario === 'quick-preset') {
    await page.locator('[data-range-preset="last_30_days"]').click();

    for (const boundary of ['start', 'end']) {
      const button = page.locator(`#${boundary}TimeButton`);
      if (await button.isDisabled()) {
        throw new Error(`${boundary} time must remain editable after choosing a quick range`);
      }
      if (await button.textContent() !== '00:00:00') {
        throw new Error(`last 30 days ${boundary} must use a midnight boundary, got ${await button.textContent()}`);
      }
    }

    await page.locator('#startTimeButton').click();
    await setTimePickerValue(page, 'start', { hour: '01', minute: '02', second: '03' });
    const confirmedResponse = page.waitForResponse((response) => {
      const url = new URL(response.url());
      return url.pathname === `${resourceBase}/stats/initial`
        && url.searchParams.get('start') === '2026-07-25T01:02:03.000Z';
    });
    await page.locator('#confirmDateRange').click();
    const confirmed = new URL((await confirmedResponse).url());
    if (confirmed.searchParams.get('end') !== '2026-08-24T00:00:00.000Z') {
      throw new Error(`expected manually edited quick range end=2026-08-24T00:00:00.000Z, got ${confirmed.searchParams.get('end')}`);
    }
  } else if (scenario === 'reverse') {
    await page.locator('[data-date="2026-08-23"]').click();
    await page.locator('[data-date="2026-08-21"]').click();
  } else {
    await page.locator('[data-date="2026-08-21"]').click();
    await page.locator('[data-date="2026-08-23"]').click();
  }

  if (scenario === 'quick-preset') {
    // Verified above.
  } else if (scenario === 'exclusive') {
    const selectedEnd = page.locator('[data-date="2026-08-23"].range-end');
    if (await selectedEnd.count() !== 1) {
      throw new Error('selecting 2026-08-21 through 2026-08-23 must mark 2026-08-23 as .range-end');
    }

    const confirmedResponse = page.waitForResponse((response) => {
      const url = new URL(response.url());
      return url.pathname === `${resourceBase}/stats/initial`
        && url.searchParams.get('start') === '2026-08-21T00:00:00.000Z';
    });
    await page.locator('#confirmDateRange').click();
    const confirmed = new URL((await confirmedResponse).url());
    if (confirmed.searchParams.get('start') !== '2026-08-21T00:00:00.000Z') {
      throw new Error(`expected confirmed start=2026-08-21T00:00:00.000Z, got ${confirmed.searchParams.get('start')}`);
    }
    if (confirmed.searchParams.get('end') !== '2026-08-24T00:00:00.000Z') {
      throw new Error(`expected confirmed end=2026-08-24T00:00:00.000Z, got ${confirmed.searchParams.get('end')}`);
    }
  } else if (scenario === 'los-angeles-dst') {
    const selectedEnd = page.locator('[data-date="2026-08-23"].range-end');
    if (await selectedEnd.count() !== 1) {
      throw new Error('America/Los_Angeles selection must mark 2026-08-23 as .range-end');
    }

    const confirmedResponse = page.waitForResponse((response) => {
      const url = new URL(response.url());
      return url.pathname === `${resourceBase}/stats/initial`
        && url.searchParams.get('start') === '2026-08-21T07:00:00.000Z';
    });
    await page.locator('#confirmDateRange').click();
    const confirmed = new URL((await confirmedResponse).url());
    if (confirmed.searchParams.get('start') !== '2026-08-21T07:00:00.000Z') {
      throw new Error(`expected America/Los_Angeles start=2026-08-21T07:00:00.000Z, got ${confirmed.searchParams.get('start')}`);
    }
    if (confirmed.searchParams.get('end') !== '2026-08-24T07:00:00.000Z') {
      throw new Error(`expected America/Los_Angeles end=2026-08-24T07:00:00.000Z, got ${confirmed.searchParams.get('end')}`);
    }
  } else if (scenario === 'end-time' || scenario === 'end-time-reset') {
    await page.locator('#endTimeButton').click();
    await setTimePickerValue(page, 'end', { hour: '12', minute: '00', second: '00' });

    if (scenario === 'end-time-reset') {
      await setTimePickerValue(page, 'end', { hour: '00', minute: '00', second: '00' });
    }

    const selectedEnd = page.locator('[data-date="2026-08-23"].range-end');
    if (await selectedEnd.count() !== 1) {
      throw new Error(scenario === 'end-time'
        ? 'setting end time to 12:00:00 must keep 2026-08-23 as .range-end'
        : 'resetting end time from 12:00:00 to 00:00:00 must keep 2026-08-23 as .range-end');
    }

    const confirmedResponse = page.waitForResponse((response) => {
      const url = new URL(response.url());
      return url.pathname === `${resourceBase}/stats/initial`
        && url.searchParams.get('start') === '2026-08-21T00:00:00.000Z';
    });
    await page.locator('#confirmDateRange').click();
    const confirmed = new URL((await confirmedResponse).url());
    const expectedEnd = scenario === 'end-time' ? '2026-08-23T12:00:00.000Z' : '2026-08-24T00:00:00.000Z';
    if (confirmed.searchParams.get('end') !== expectedEnd) {
      throw new Error(`expected ${scenario} end=${expectedEnd}, got ${confirmed.searchParams.get('end')}`);
    }
  } else {
    const selectedStart = page.locator('[data-date="2026-08-21"].range-start');
    if (await selectedStart.count() !== 1) {
      throw new Error('reverse selection 2026-08-23 through 2026-08-21 must mark 2026-08-21 as .range-start');
    }
    const selectedEnd = page.locator('[data-date="2026-08-23"].range-end');
    if (await selectedEnd.count() !== 1) {
      throw new Error('reverse selection 2026-08-23 through 2026-08-21 must mark 2026-08-23 as .range-end');
    }

    const confirmedResponse = page.waitForResponse((response) => {
      const url = new URL(response.url());
      return url.pathname === `${resourceBase}/stats/initial`
        && url.searchParams.get('start') === '2026-08-21T00:00:00.000Z';
    });
    await page.locator('#confirmDateRange').click();
    const confirmed = new URL((await confirmedResponse).url());
    if (confirmed.searchParams.get('end') !== '2026-08-24T00:00:00.000Z') {
      throw new Error(`expected reverse-selection end=2026-08-24T00:00:00.000Z, got ${confirmed.searchParams.get('end')}`);
    }
  }
  }
  if (pageErrors.length) {
    throw pageErrors[0];
  }
} finally {
  await browser.close();
  await new Promise((resolve, reject) => server.close((error) => error ? reject(error) : resolve()));
}
