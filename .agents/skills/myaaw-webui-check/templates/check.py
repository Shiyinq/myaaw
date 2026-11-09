"""Throwaway browser harness for the Myaaw dashboard.

Copy to `.tmp-check.py` at the repo root, set PAGES, run with the repo venv:

    .venv/bin/python .tmp-check.py

It logs in through the dev server's `/api` proxy, seeds the client state, then
prints geometry and any horizontally clipped element for every page at a desktop
and a phone viewport. Delete it (and its screenshots) when you are done.
"""

import json
import time
import urllib.request

from selenium import webdriver
from selenium.webdriver.chrome.options import Options as ChromeOptions
from selenium.webdriver.firefox.options import Options as FirefoxOptions

APP = "http://localhost:5173"
LOGIN = {"username": "myaaw", "password": "myaawadmin"}
AGENT = "default"
LOCALE = "en"

# Edit me: routes to inspect, relative to APP.
PAGES = ["/app/tools", "/app/skills", "/app/context"]

DESKTOP = (1440, 900)
PHONE = (375, 812)  # iPhone-ish; Chrome mobile emulation only


def login() -> dict:
    request = urllib.request.Request(
        APP + "/api/auth/login",
        data=json.dumps(LOGIN).encode(),
        headers={"Content-Type": "application/json"},
    )
    return json.load(urllib.request.urlopen(request))


def seed(driver, auth: dict) -> None:
    """Fill the keys the app reads on boot; everything lives in localStorage."""
    driver.get(APP + "/login")
    driver.execute_script(
        """
        localStorage.setItem('myaaw_auth', 'true');
        localStorage.setItem('myaaw_token', arguments[0]);
        localStorage.setItem('myaaw_token_expires', arguments[1]);
        localStorage.setItem('myaaw_agent', arguments[2]);
        localStorage.setItem('app_locale', arguments[3]);
        localStorage.setItem('myaaw_sidebar_collapsed', 'false');
        localStorage.setItem('myaaw_chat_history_open', 'false');
        document.cookie = 'app_locale=' + arguments[3] + '; path=/';
        """,
        auth["token"],
        auth["expires_at"],
        AGENT,
        LOCALE,
    )


def chrome(mobile: bool = False) -> webdriver.Chrome:
    options = ChromeOptions()
    options.add_argument("--headless=new")
    options.add_argument("--no-sandbox")
    options.add_argument("--disable-gpu")
    if mobile:
        width, height = PHONE
        options.add_experimental_option(
            "mobileEmulation",
            {
                "deviceMetrics": {"width": width, "height": height, "pixelRatio": 2},
                "userAgent": (
                    "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) "
                    "AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 "
                    "Mobile/15E148 Safari/604.1"
                ),
            },
        )
    else:
        options.add_argument(f"--window-size={DESKTOP[0]},{DESKTOP[1]}")
    return webdriver.Chrome(options=options)


def firefox(width: int = DESKTOP[0], height: int = DESKTOP[1]) -> webdriver.Firefox:
    options = FirefoxOptions()
    options.add_argument("-headless")
    driver = webdriver.Firefox(options=options)
    driver.set_window_size(width, height)
    return driver


# Report geometry: the page scrollers, the header, and every element that
# escapes the viewport horizontally (the usual cause of "content is cut off").
MEASURE = """
const main = document.querySelector('main');
const scrollers = [...main.querySelectorAll('*')]
  .filter((el) => ['auto', 'scroll'].includes(getComputedStyle(el).overflowY))
  .map((el) => {
    const r = el.getBoundingClientRect();
    return {
      cls: el.className.toString().slice(0, 60),
      top: Math.round(r.top), h: Math.round(r.height),
      sh: el.scrollHeight, ch: el.clientHeight,
      scrolls: el.scrollHeight > el.clientHeight + 4,
      at: Math.round(el.scrollTop),
    };
  });
const clipped = [...main.querySelectorAll('*')]
  // A closed drawer sits off-screen on purpose; it is inert, so ignore it.
  .filter((el) => !el.closest('[inert]'))
  .map((el) => ({ el, r: el.getBoundingClientRect() }))
  .filter(({ r }) => r.width > 0 && r.height > 0 && (r.left < -1 || r.right > innerWidth + 1))
  .slice(0, 5)
  .map(({ el, r }) => ({
    cls: el.className.toString().slice(0, 60),
    left: Math.round(r.left), right: Math.round(r.right), w: Math.round(r.width),
    text: (el.textContent || '').trim().slice(0, 40),
  }));
return {
  vw: innerWidth, vh: innerHeight,
  title: document.title,
  header: (() => {
    const h = document.querySelector('header');
    if (!h) return null;
    const r = h.getBoundingClientRect();
    const buttons = [...h.querySelectorAll('button')]
      .filter((b) => b.getBoundingClientRect().width > 0)
      .map((b) => b.getAttribute('aria-label') || b.textContent.trim().slice(0, 16));
    return { h: Math.round(r.height), buttons };
  })(),
  pageOverflowX: document.documentElement.scrollWidth - innerWidth,
  scrollers,
  clipped,
};
"""


def run(browser_name: str, factory, label: str, auth: dict) -> None:
    driver = factory()
    try:
        seed(driver, auth)
        for path in PAGES:
            driver.get(APP + path)
            # Wait for the page's own data fetch; bump this for slow pages.
            time.sleep(4)
            report = driver.execute_script(MEASURE)
            print(f"\n{label} {path}")
            print(json.dumps(report, indent=1)[:2500])
            driver.save_screenshot(f"/tmp/{browser_name}{path.replace('/', '-')}.png")
    finally:
        driver.quit()


if __name__ == "__main__":
    auth = login()
    run("chrome-desktop", lambda: chrome(False), "desktop 1440x900", auth)
    run("chrome-phone", lambda: chrome(True), "phone 375x812", auth)
    # A second engine, for CSS/HMR differences and keyboard checks.
    run("firefox", firefox, "firefox 1440x900", auth)
