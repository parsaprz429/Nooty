<?php

declare(strict_types=1);

define('DEFAULT_MAX_RESULTS', 30);
define('DEFAULT_TIMEOUT', 4);        // ⚡ تایم‌اوت بهینه برای سرعت بالا
define('CONNECT_TIMEOUT', 2);        // ⚡ اتصال سریع

$query = trim($_GET['q'] ?? '');
$results = [];
$search_time = 0;
$error = '';

if ($query !== '' && mb_strlen($query) >= 2) {

    class UltimateSearchEngine {
        private array $engines = [
            'Google' => [
                'url' => 'https://www.google.com/search?q=%s&hl=fa&num=15',
                'parser' => 'regex',
                'pattern' => '/<a[^>]+href="\/url\?q=([^&"]+)[^"]*"[^>]*><h3[^>]*>(.*?)<\/h3>/sS',
                'icon' => '🔎'
            ],
            'DuckDuckGo' => [
                'url' => 'https://html.duckduckgo.com/html/?q=%s&kl=ir-fa',
                'parser' => 'regex',
                'pattern' => '/<a[^>]*class="result__a"[^>]*href="([^"]+)"[^>]*>(.*?)<\/a>/sS',
                'icon' => '🦆'
            ],
            'Yahoo' => [
                'url' => 'https://search.yahoo.com/search?p=%s',
                'parser' => 'regex',
                'pattern' => '/<a[^>]*class="[^"]*ac-algo[^"]*"[^>]*href="([^"]+)"[^>]*>(.*?)<\/a>/sS',
                'icon' => '🅨'
            ],
            'ویکی‌پدیا' => [
                'url' => 'https://fa.wikipedia.org/w/api.php?action=query&list=search&srsearch=%s&format=json&srlimit=6',
                'parser' => 'wikipedia',
                'icon' => '📚'
            ],
        ];

        public function search(string $query): array {
            $all = [];
            $mh = curl_multi_init();
            $handles = [];

            foreach ($this->engines as $engine => $config) {
                $url = sprintf($config['url'], urlencode($query));
                $ch = curl_init($url);
                curl_setopt_array($ch, [
                    CURLOPT_RETURNTRANSFER => true,
                    CURLOPT_FOLLOWLOCATION => true,
                    CURLOPT_TIMEOUT => DEFAULT_TIMEOUT,
                    CURLOPT_CONNECTTIMEOUT => CONNECT_TIMEOUT,
                    CURLOPT_SSL_VERIFYPEER => false,
                    CURLOPT_ENCODING => 'gzip, deflate',
                    CURLOPT_USERAGENT => 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36',
                    CURLOPT_REFERER => 'https://www.google.com/',
                    CURLOPT_HTTPHEADER => [
                        'Accept: text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8',
                        'Accept-Language: fa-IR,fa;q=0.9,en-US;q=0.8,en;q=0.7'
                    ],
                ]);
                curl_multi_add_handle($mh, $ch);
                $handles[] = ['ch' => $ch, 'engine' => $engine, 'config' => $config];
            }

            $running = null;
            do {
                curl_multi_exec($mh, $running);
                if ($running) curl_multi_select($mh, 0.02);
            } while ($running > 0);

            foreach ($handles as $h) {
                $content = curl_multi_getcontent($h['ch']);
                if (!empty($content)) {
                    $all = array_merge($all, $this->parseContent($content, $h['config'], $h['engine']));
                }
                curl_multi_remove_handle($mh, $h['ch']);
                curl_close($h['ch']);
            }
            curl_multi_close($mh);

            return $this->processResults($all, $query);
        }

        private function parseContent(string $content, array $config, string $engine): array {
            return match ($config['parser']) {
                'wikipedia' => $this->parseWikipedia($content),
                'regex'     => $this->parseRegex($content, $config['pattern'], $engine, $config['icon'] ?? '🔍'),
                default     => [],
            };
        }

        private function parseWikipedia(string $json): array {
            $data = json_decode($json, true);
            $out = [];
            foreach ($data['query']['search'] ?? [] as $item) {
                $out[] = [
                    'title' => $item['title'],
                    'link' => 'https://fa.wikipedia.org/wiki/' . urlencode(str_replace(' ', '_', $item['title'])),
                    'snippet' => strip_tags($item['snippet'] ?? ''),
                    'engine' => 'ویکی‌پدیا',
                    'icon' => '📚'
                ];
            }
            return $out;
        }

        private function parseRegex(string $html, string $pattern, string $engine, string $icon): array {
            $out = [];
            if (preg_match_all($pattern, $html, $matches, PREG_SET_ORDER)) {
                foreach (array_slice($matches, 0, 12) as $m) {
                    $link = html_entity_decode($m[1]);
                    if ($engine === 'Google') $link = urldecode($link);
                    $out[] = [
                        'title' => trim(strip_tags($m[2] ?? '')),
                        'link' => $link,
                        'snippet' => '',
                        'engine' => $engine,
                        'icon' => $icon
                    ];
                }
            }
            return $out;
        }

        private function processResults(array $results, string $query): array {
            $processed = [];
            $seen = [];
            foreach ($results as $r) {
                if (empty($r['title']) || empty($r['link'])) continue;
                $u = $this->processUrl($r['link']);
                if (!$u['domain']) continue;
                $key = md5($u['url']);
                if (isset($seen[$key])) continue;
                $seen[$key] = true;

                $processed[] = [
                    'title' => htmlspecialchars($r['title'], ENT_QUOTES, 'UTF-8'),
                    'url' => $u['url'],
                    'domain' => $u['domain'],
                    'display_domain' => $u['display_domain'],
                    'category' => $u['category'],
                    'icon' => $u['icon'],
                    'secure' => $u['secure'] ? '🔒' : '🌐',
                    'snippet' => htmlspecialchars($r['snippet'] ?? '', ENT_QUOTES, 'UTF-8'),
                    'engine' => $r['engine'],
                    'engine_icon' => $r['icon'] ?? '🔍',
                    'score' => min(100, round($this->calculateScore($r['title'], $query), 1)),
                ];
            }
            usort($processed, fn($a, $b) => $b['score'] <=> $a['score']);
            return array_slice($processed, 0, DEFAULT_MAX_RESULTS);
        }

        private function processUrl(string $url): array {
            if (str_contains($url, '//duckduckgo.com/l/?uddg=')) {
                parse_str(parse_url($url, PHP_URL_QUERY) ?? '', $p);
                if (!empty($p['uddg'])) $url = urldecode($p['uddg']);
            }
            $url = preg_replace('/([?&])(utm_[^&]+|gclid|fbclid|msclkid|dclid)=[^&]*&?/', '$1', $url);
            $url = rtrim($url, '?&');
            $domain = preg_replace('/^www\./', '', parse_url($url, PHP_URL_HOST) ?? '');

            $category = 'عمومی'; $icon = '🌐';
            if (stripos($domain, 'wikipedia.org') !== false)        { $category = 'دانشنامه'; $icon = '📚'; }
            elseif (stripos($domain, 'github.com') !== false)       { $category = 'کد';        $icon = '💻'; }
            elseif (stripos($domain, 'stackoverflow.com') !== false){ $category = 'توسعه';     $icon = '⚙️'; }
            elseif (stripos($domain, 'youtube.com') !== false)      { $category = 'ویدیو';     $icon = '🎬'; }
            elseif (str_ends_with($domain, '.ir'))                  { $category = 'ایرانی';    $icon = '🇮🇷'; }

            return [
                'url' => $url,
                'domain' => $domain,
                'display_domain' => mb_substr($domain, 0, 32),
                'category' => $category,
                'icon' => $icon,
                'secure' => str_starts_with($url, 'https://')
            ];
        }

        private function calculateScore(string $title, string $query): float {
            $score = 0.0;
            $t = mb_strtolower($title);
            $q = mb_strtolower($query);
            if (str_contains($t, $q)) $score += 30.0;
            foreach (explode(' ', $q) as $term) {
                if (mb_strlen($term) > 2 && str_contains($t, $term)) $score += 10.0;
            }
            return min(100.0, $score);
        }
    }

    $start = microtime(true);
    try {
        $results = (new UltimateSearchEngine())->search($query);
        $search_time = (int) round((microtime(true) - $start) * 1000);
    } catch (Throwable $e) {
        $error = 'خطا در برقراری ارتباط با سرویس‌های موتور جستجو. لطفا دوباره تلاش کنید.';
    }
}
$is_search_page = !empty($query);
?>
<!DOCTYPE html>
<html lang="fa" dir="rtl">
<head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0, maximum-scale=5.0, viewport-fit=cover" />
    <title><?= $is_search_page ? htmlspecialchars($query) . ' - NootyPlus Search' : 'NootyPlus • جستجوی هوشمند و امن' ?></title>
    
    <!-- SEO & Metadata -->
    <meta name="description" content="موتور جستجوی اختصاصی، امن و پرسرعت اکوسیستم نوتی با هوش مصنوعی و آنالیز همزمان چند موتور جستجو.">
    <meta name="author" content="Nooty Ecosystem">
    <meta name="theme-color" content="#090a0f">

    <!-- Open Graph -->
    <meta property="og:type" content="website">
    <meta property="og:title" content="NootyPlus Search">
    <meta property="og:description" content="جستجوی سریع، ناشناس و جامع در وب بدون تبلیغات مزاحم.">
    <meta property="og:image" content="https://nooty.ir/logo.png">

    <!-- Favicon -->
    <link rel="icon" type="image/png" href="https://nooty.ir/logo.png" id="favicon">

    <style>
        /* Smart Font Loader with Fallback */
        @font-face {
            font-family: 'Vazirmatn';
            src: url('https://ai.nooty.ir/libs/fonts/Vazirmatn%5Bwght%5D.woff2') format('woff2-variations'),
                 url('/ai/libs/fonts/Vazirmatn%5Bwght%5D.woff2') format('woff2-variations');
            font-weight: 100 900;
            font-style: normal;
            font-display: swap;
        }

        :root {
            --bg: #090a0f;
            --bg-header: rgba(18, 20, 29, 0.75);
            --bg-card: rgba(255, 255, 255, 0.03);
            --border: rgba(255, 255, 255, 0.08);
            --border-hover: rgba(138, 180, 248, 0.3);
            --text: #f1f5f9;
            --text-muted: #94a3b8;
            --accent: #8ab4f8;
            --accent-glow: rgba(138, 180, 248, 0.15);
            --url-color: #34d399;
            --radius-sm: 10px;
            --radius-lg: 24px;
            --radius-pill: 999px;
            --shadow-soft: 0 10px 30px rgba(0, 0, 0, 0.5);
            --ease: cubic-bezier(0.16, 1, 0.3, 1);
        }
        
        * { margin: 0; padding: 0; box-sizing: border-box; -webkit-tap-highlight-color: transparent; }
        
        body {
            font-family: 'Vazirmatn', system-ui, -apple-system, BlinkMacSystemFont, sans-serif;
            background-color: var(--bg);
            background-image: radial-gradient(circle at 50% 0%, rgba(138, 180, 248, 0.08), transparent 50%);
            color: var(--text);
            min-height: 100vh;
            display: flex;
            flex-direction: column;
            line-height: 1.6;
        }
        
        /* Navbar & Header */
        .header {
            position: sticky; top: 0; z-index: 100;
            background: var(--bg-header);
            backdrop-filter: blur(20px);
            -webkit-backdrop-filter: blur(20px);
            border-bottom: 1px solid var(--border);
            padding: 14px 24px; display: flex; align-items: center; gap: 24px;
            box-shadow: var(--shadow-soft);
            transition: all 0.3s var(--ease);
        }
        
        .brand { display: flex; align-items: center; gap: 10px; text-decoration: none; color: var(--text); }
        .brand-logo { 
            width: 36px; height: 36px; border-radius: 10px; object-fit: cover;
            background: rgba(255, 255, 255, 0.05); border: 1px solid var(--border);
        }
        .brand-text { font-size: 1.3rem; font-weight: 800; letter-spacing: -0.5px; }
        .brand-text span { color: var(--accent); }
        
        /* Search Bar */
        .search-form { flex: 1; max-width: 680px; }
        .search-box {
            display: flex; align-items: center; background: rgba(255, 255, 255, 0.04);
            border: 1px solid var(--border); border-radius: var(--radius-pill);
            padding: 0 16px; height: 48px; transition: all 0.25s var(--ease);
        }
        .search-box:focus-within {
            background: rgba(255, 255, 255, 0.07);
            border-color: var(--accent);
            box-shadow: 0 0 0 3px var(--accent-glow);
        }
        .search-input {
            flex: 1; background: transparent; border: none; outline: none; color: white; 
            font-size: 1rem; font-family: inherit; padding: 0 12px; width: 100%;
        }
        .search-btn {
            background: transparent; border: none; color: var(--accent); cursor: pointer;
            display: grid; place-items: center; padding: 8px; border-radius: 50%;
            transition: background 0.2s;
        }
        .search-btn:hover { background: var(--accent-glow); }
        
        /* Main Container */
        .main-content {
            flex: 1; padding: 28px 24px; display: flex; flex-direction: column; align-items: flex-start;
            max-width: 820px; margin: 0 auto; width: 100%;
        }
        
        /* Stats Label */
        .stats { color: var(--text-muted); font-size: 0.88rem; margin-bottom: 24px; font-weight: 500; }
        
        /* Results Card List */
        .results { display: flex; flex-direction: column; gap: 24px; width: 100%; }
        
        .result-item { 
            display: flex; flex-direction: column; gap: 6px; 
            background: var(--bg-card);
            border: 1px solid var(--border);
            padding: 20px; border-radius: var(--radius-lg);
            transition: all 0.25s var(--ease);
            animation: fadeIn 0.4s var(--ease) forwards; 
        }

        .result-item:hover {
            border-color: var(--border-hover);
            transform: translateY(-2px);
            box-shadow: 0 10px 25px rgba(0, 0, 0, 0.3);
        }
        
        .result-url-wrap { display: flex; align-items: center; gap: 8px; font-size: 0.85rem; color: var(--text-muted); }
        .result-icon { width: 24px; height: 24px; background: rgba(255,255,255,0.05); border-radius: 50%; display: grid; place-items: center; font-size: 0.8rem; }
        .result-url { color: var(--url-color); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; max-width: 75%; font-family: monospace; }
        
        .result-title { 
            font-size: 1.2rem; font-weight: 700; color: var(--accent); text-decoration: none; 
            line-height: 1.4; display: inline-block; transition: color 0.2s;
        }
        .result-title:hover { text-decoration: underline; color: #a6c8ff; }
        
        .result-snippet { font-size: 0.92rem; color: var(--text); line-height: 1.7; word-wrap: break-word; opacity: 0.9; }
        
        .meta-tag { background: rgba(255,255,255,0.06); padding: 3px 10px; border-radius: 6px; font-size: 0.78rem; color: var(--text-muted); }

        /* Error Box */
        .error-box { 
            background: rgba(239, 68, 68, 0.1); color: #f87171; padding: 20px; 
            border-radius: var(--radius-lg); border: 1px solid rgba(239, 68, 68, 0.2); width: 100%; 
        }
        
        /* Top Loading Bar */
        .loading-bar {
            position: fixed; top: 0; left: 0; right: 0; height: 3px; background: var(--accent);
            z-index: 1000; transform-origin: left; transform: scaleX(0); transition: transform 0.2s ease;
            display: none;
        }
        .loading-bar.active { display: block; animation: loading 1.8s cubic-bezier(0.4, 0, 0.2, 1) infinite; }
        
        @keyframes fadeIn { from { opacity: 0; transform: translateY(12px); } to { opacity: 1; transform: none; } }
        @keyframes loading { 0% { transform: scaleX(0); } 50% { transform: scaleX(0.7); } 100% { transform: scaleX(1); } }
        
        /* Footer */
        .footer {
            text-align: center; font-size: 0.8rem; color: var(--text-muted);
            padding: 30px 20px; margin-top: auto; border-top: 1px solid var(--border);
            display: flex; flex-direction: column; gap: 6px;
        }
        .footer a { color: var(--accent); text-decoration: none; font-weight: 600; }
        .footer a:hover { text-decoration: underline; }

        /* Responsive Breakpoints */
        @media (max-width: 768px) {
            .header { flex-direction: column; align-items: stretch; gap: 14px; padding: 16px; }
            .brand { justify-content: center; }
            .search-form { max-width: 100%; }
            .main-content { padding: 18px 16px; }
            .result-title { font-size: 1.1rem; }
        }
        
        /* Home Mode (Initial Search Page) */
        .home-mode .header { 
            background: transparent; border: none; box-shadow: none; flex-direction: column;
            justify-content: center; min-height: 65vh; gap: 32px; position: relative;
        }
        .home-mode .brand-logo { width: 64px; height: 64px; border-radius: 18px; }
        .home-mode .brand-text { font-size: 2.6rem; }
        .home-mode .search-form { max-width: 620px; width: 100%; margin: 0 auto; }
        .home-mode .search-box { height: 56px; padding: 0 20px; }
        .home-mode .footer { border-top: none; }
    </style>
</head>
<body class="<?= !$is_search_page ? 'home-mode' : '' ?>">

    <!-- ⚡ Top Speed Loading Indicator -->
    <div class="loading-bar" id="loadingBar"></div>

    <!-- Header Navbar -->
    <header class="header">
        <a href="./" class="brand" aria-label="NootyPlus Search">
            <img src="https://nooty.ir/logo.png" alt="Nooty" class="brand-logo" id="brandLogo">
            <span class="brand-text">Nooty<span>Plus</span></span>
        </a>
        
        <form class="search-form" method="get" action="" id="searchForm">
            <div class="search-box">
                <input type="text" class="search-input" name="q" placeholder="جستجوی سریع، امن و ناشناس نوتی..." value="<?= htmlspecialchars($query) ?>" autofocus autocomplete="off" required />
                <button type="submit" class="search-btn" aria-label="Search">
                    <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><circle cx="11" cy="11" r="7"/><path d="M20 20l-3.5-3.5"/></svg>
                </button>
            </div>
        </form>
    </header>

    <!-- Results Container -->
    <?php if ($is_search_page): ?>
    <main class="main-content">
        <?php if ($error): ?>
            <div class="error-box">⚠️ <?= htmlspecialchars($error) ?></div>
        <?php elseif (!empty($results)): ?>
            
            <div class="stats">
                حدود <?= count($results) ?> نتیجه (<?= $search_time ?> میلی‌ثانیه) ⚡
            </div>

            <div class="results">
                <?php foreach ($results as $i => $res): ?>
                <article class="result-item" style="animation-delay: <?= $i * 25 ?>ms;">
                    <div class="result-url-wrap">
                        <span class="result-icon"><?= $res['icon'] ?></span>
                        <span class="result-url"><?= $res['secure'] ?> <?= $res['display_domain'] ?></span>
                        <span class="meta-tag"><?= $res['engine_icon'] ?> <?= $res['engine'] ?></span>
                    </div>
                    <a href="<?= $res['url'] ?>" target="_blank" rel="noopener noreferrer" class="result-title">
                        <?= $res['title'] ?>
                    </a>
                    <?php if (!empty($res['snippet'])): ?>
                    <div class="result-snippet">
                        <?= mb_strlen($res['snippet']) > 220 ? mb_substr($res['snippet'], 0, 220) . '...' : $res['snippet'] ?>
                    </div>
                    <?php endif; ?>
                </article>
                <?php endforeach; ?>
            </div>

        <?php else: ?>
            <div class="error-box" style="background: rgba(255,255,255,0.03); color: var(--text); border-color: var(--border);">
                <h3 style="margin-bottom: 8px;">نتیجه‌ای یافت نشد 🔍</h3>
                <p>برای عبارت «<strong><?= htmlspecialchars($query) ?></strong>» متأسفانه نتیجه‌ای پیدا نشد. املای کلمات را بررسی کرده یا عبارت دیگری را جستجو کنید.</p>
            </div>
        <?php endif; ?>
    </main>
    <?php endif; ?>

    <!-- Footer -->
    <footer class="footer">
        <div>© 2026 Nooty Ecosystem — <a href="https://nooty.ir" target="_blank" rel="noopener">nooty.ir</a></div>
        <div style="opacity:0.6;">جستجوگر هوشمند چندکاناله با تضمین حریم خصوصی</div>
    </footer>

    <script>
    (function() {
        'use strict';

        const form = document.getElementById('searchForm');
        const loadingBar = document.getElementById('loadingBar');
        const brandLogo = document.getElementById('brandLogo');

        // SVG Fallback for Logo
        const DEFAULT_LOGO_SVG = 'data:image/svg+xml;utf8,<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100" fill="%238ab4f8"><rect width="100" height="100" rx="20"/><path d="M30 70V30l40 40V30" stroke="%23fff" stroke-width="10" stroke-linecap="round" stroke-linejoin="round" fill="none"/></svg>';

        if (brandLogo) {
            brandLogo.onerror = () => {
                if (brandLogo.src !== DEFAULT_LOGO_SVG) brandLogo.src = DEFAULT_LOGO_SVG;
            };
        }

        if (form && loadingBar) {
            form.addEventListener('submit', () => {
                const val = form.querySelector('.search-input').value.trim();
                if (val.length >= 2) {
                    loadingBar.classList.add('active');
                }
            });
        }

        window.addEventListener('pageshow', () => {
            if (loadingBar) loadingBar.classList.remove('active');
        });
    })();
    </script>
</body>
</html>
