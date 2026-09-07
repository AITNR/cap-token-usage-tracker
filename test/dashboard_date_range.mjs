import { createServer } from 'node:http';
import { readFile } from 'node:fs/promises';
import { chromium } from 'playwright-core';

const [htmlPath, chromePath, scenario = 'exclusive'] = process.argv.slice(2);
if (!htmlPath || !chromePath) {
  throw new Error('usage: node test/dashboard_date_range.mjs <dashboard-html-path> <google-chrome-path> [exclusive|end-time|end-time-reset|quick-preset|reverse|los-angeles-dst|token-unit]');
}
if (!['exclusive', 'end-time', 'end-time-reset', 'quick-preset', 'reverse', 'los-angeles-dst', 'token-unit'].includes(scenario)) {
  throw new Error(`unknown dashboard date-range browser scenario: ${scenario}`);
}

const dashboardHTML = await readFile(htmlPath);
const resourceBase = '/v0/resource/plugins/calendar-browser-test';
const timezoneId = scenario === 'los-angeles-dst' ? 'America/Los_Angeles' : 'UTC';
const initialRange = scenario === 'los-angeles-dst'
  ? { start: '2026-08-23T07:00:00.000Z', end: '2026-08-24T07:00:00.000Z' }
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
const initialPayload = scenario === 'token-unit' ? tokenUnitInitial : emptyInitial;
const savedTokenDisplayModes = [];

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
      sendJSON({
        request_page_size: url.searchParams.get('request_page_size'),
        dimension_page_size: url.searchParams.get('dimension_page_size'),
        time_range_mode: url.searchParams.get('time_range_mode'),
        token_display_mode: url.searchParams.get('token_display_mode'),
      });
      return;
    }
    sendJSON({
      time_range_mode: 'custom',
      time_range_start: initialRange.start,
      time_range_end: initialRange.end,
      token_display_mode: scenario === 'token-unit' ? 'B' : 'full',
    });
    return;
  }
  if (url.pathname === `${resourceBase}/stats/initial`) {
    sendJSON(initialPayload);
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
const dashboardURL = `http://127.0.0.1:${address.port}${resourceBase}/dashboard`;
const browser = await chromium.launch({ executablePath: chromePath, headless: true });

try {
  const context = await browser.newContext({ timezoneId });
  const page = await context.newPage();
  const pageErrors = [];
  page.on('pageerror', (error) => pageErrors.push(error));

  await page.clock.install({ time: new Date('2026-08-23T12:00:00.000Z') });
  await page.goto(dashboardURL, { waitUntil: 'domcontentloaded' });
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
    if (JSON.stringify(savedTokenDisplayModes) !== JSON.stringify(['full', 'k', 'm', 'B'])) {
      throw new Error(`expected saved token display modes full,k,m,B, got ${savedTokenDisplayModes.join(',')}`);
    }
    if (await tokenButton.textContent() !== 'B' || await totalTokens.textContent() !== '1.23B') {
      throw new Error(`expected token unit cycle to return to B, got ${await tokenButton.textContent()} / ${await totalTokens.textContent()}`);
    }
  }

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
  if (pageErrors.length) {
    throw pageErrors[0];
  }
} finally {
  await browser.close();
  await new Promise((resolve, reject) => server.close((error) => error ? reject(error) : resolve()));
}
