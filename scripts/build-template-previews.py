"""Build standalone vector illustrations of the three bundled theme layouts."""
from pathlib import Path
from html import escape
import xml.etree.ElementTree as ET
from base64 import b64encode

out = Path(__file__).resolve().parents[1] / 'internal/httpui/assets'

def text(x, y, value, size=20, fill='#29251e', font='system-ui, -apple-system, sans-serif', weight='400'):
    return f'<text x="{x}" y="{y}" font-family="{font}" font-size="{size}" font-weight="{weight}" fill="{fill}">{escape(value)}</text>'

def line(x, y, w, color):
    return f'<path d="M{x} {y}h{w}" stroke="{color}"/>'

def rect(x, y, w, h, fill, r=0, stroke=None, stroke_width=1):
    st = f' stroke="{stroke}" stroke-width="{stroke_width}"' if stroke else ''
    return f'<rect x="{x}" y="{y}" width="{w}" height="{h}" rx="{r}" fill="{fill}"{st}/>'

def svg(name, parts):
    data = '<svg xmlns="http://www.w3.org/2000/svg" width="1280" height="800" viewBox="0 0 1280 800">' + ''.join(parts) + '</svg>'
    ET.fromstring(data)
    assert 'foreignObject' not in data and '<script' not in data
    (out / f'template-{name}.svg').write_text(data)

# Read vector path for GoPress logo
gopress_raw = (out / 'gopress.svg').read_text()
g_start = gopress_raw.find('<g fill="#fff">') + len('<g fill="#fff">')
g_end = gopress_raw.rfind('</g>')
gopress_paths = gopress_raw[g_start:g_end]

# -------------------------------------------------------------
# 1. TEMPLATE 1: БЛОГ (Journal - Editorial Magazine in Merinda Style)
# -------------------------------------------------------------
photo = b64encode((out / 'journal-cover-1.webp').read_bytes()).decode()
logo_journal = f'<g transform="translate(56, 32) scale(0.062)" fill="#111827">{gopress_paths}</g>'
p_journal = [
    rect(0, 0, 1280, 800, '#ffffff'),
    logo_journal,
    text(860, 56, 'Главная     Категории     Поиск     RSS', 13, fill='#4b5563', weight='500'),
    line(56, 80, 1168, '#e5e7eb'),
    text(56, 110, 'Все публикации', 13, fill='#111827', weight='700'),
    line(56, 122, 105, '#111827'),
    text(190, 110, 'Дизайн          Культура          Технологии          Практика', 13, fill='#6b7280', weight='500'),
    line(56, 123, 1168, '#e5e7eb'),
    # Spotlight Story (Left)
    rect(56, 146, 730, 230, '#f9fafb', 6),
    f'<image x="56" y="146" width="730" height="230" preserveAspectRatio="xMidYMid slice" href="data:image/webp;base64,{photo}"/>',
    rect(56, 394, 150, 22, '#ecfdf5', 4),
    text(66, 409, 'ДИЗАЙН · 27.10.2026', 10, fill='#03a87c', weight='700'),
    text(56, 444, 'Меньше шума. Больше смысла.', 28, fill='#111827', weight='700'),
    text(56, 472, 'Как создавать вещи с характером и оставлять место для главного в цифровом мире.', 14, fill='#4b5563'),
    text(56, 498, '5 мин чтения · Редакция', 12, fill='#9ca3af'),
    # Trending Column (Right)
    text(826, 166, 'ПОПУЛЯРНОЕ В ЖУРНАЛЕ', 12, fill='#111827', weight='700'),
    line(826, 180, 398, '#111827'),
    text(826, 226, '01', 34, fill='#9ca3af', weight='800'),
    text(876, 210, 'Искусство делать паузу', 16, fill='#111827', weight='600'),
    text(876, 230, '27.10.2026 · 5 мин чтения', 12, fill='#6b7280'),
    line(826, 252, 398, '#e5e7eb'),
    text(826, 298, '02', 34, fill='#9ca3af', weight='800'),
    text(876, 282, 'Красота простых вещей', 16, fill='#111827', weight='600'),
    text(876, 302, '26.10.2026 · 4 мин чтения', 12, fill='#6b7280'),
    line(826, 324, 398, '#e5e7eb'),
    text(826, 370, '03', 34, fill='#9ca3af', weight='800'),
    text(876, 354, 'Как сохранить любопытство', 16, fill='#111827', weight='600'),
    text(876, 374, '25.10.2026 · 3 мин чтения', 12, fill='#6b7280'),
    line(826, 396, 398, '#e5e7eb'),
    # Section Divider
    text(56, 532, 'Свежие публикации', 18, fill='#111827', weight='700'),
    line(56, 544, 1168, '#111827'),
    # Horizontal Cards
    text(56, 578, 'ДИЗАЙН · 24.10.2026', 10, fill='#03a87c', weight='700'),
    text(56, 606, 'Новый взгляд на цифровое пространство', 18, fill='#111827', weight='700'),
    text(56, 632, 'Интерфейсы должны помогать человеку, а не отвлекать его внимание.', 13, fill='#4b5563'),
    text(56, 660, '5 мин чтения · Редакция', 11, fill='#9ca3af'),
    rect(450, 564, 130, 95, '#f3f4f6', 6),
    f'<image x="450" y="564" width="130" height="95" preserveAspectRatio="xMidYMid slice" href="data:image/webp;base64,{photo}"/>',
    line(56, 686, 524, '#e5e7eb'),
    text(650, 578, 'КУЛЬТУРА · 22.10.2026', 10, fill='#03a87c', weight='700'),
    text(650, 606, 'Привычка замечать главное вокруг', 18, fill='#111827', weight='700'),
    text(650, 632, 'Внимание к повседневным деталям меняет восприятие окружающего мира.', 13, fill='#4b5563'),
    text(650, 660, '4 мин чтения · Редакция', 11, fill='#9ca3af'),
    rect(1094, 564, 130, 95, '#f3f4f6', 6),
    f'<image x="1094" y="564" width="130" height="95" preserveAspectRatio="xMidYMid slice" href="data:image/webp;base64,{photo}"/>',
    line(650, 686, 574, '#e5e7eb'),
]
svg('journal', p_journal)

# -------------------------------------------------------------
# 2. TEMPLATE 2: ДОКУМЕНТАЦИЯ (Minimal - Technical Documentation)
# -------------------------------------------------------------
logo_docs = f'<g transform="translate(48, 36) scale(0.055)" fill="#0f172a">{gopress_paths}</g>'
p_docs = [
    rect(0, 0, 1280, 800, '#ffffff'),
    # Left Sidebar
    rect(0, 0, 280, 800, '#f8fafc'),
    line(280, 0, 0, '#e2e8f0'),
    '<path d="M280 0v800" stroke="#e2e8f0"/>',
    logo_docs,
    rect(194, 38, 68, 20, '#eff6ff', 10),
    text(204, 52, 'v1.0 · Docs', 10, fill='#2563eb', weight='600'),
    # Search Bar
    rect(48, 80, 208, 36, '#ffffff', 6, stroke='#cbd5e1'),
    text(60, 103, 'Поиск по докам...', 12, fill='#94a3b8'),
    rect(226, 88, 20, 20, '#f1f5f9', 4, stroke='#e2e8f0'),
    text(233, 102, '/', 11, fill='#64748b', weight='600'),
    # Nav items
    text(48, 148, 'РУКОВОДСТВА', 11, fill='#94a3b8', weight='700'),
    rect(40, 162, 224, 34, '#eff6ff', 6),
    line(40, 162, 0, '#2563eb'),
    '<path d="M40 162v34" stroke="#2563eb" stroke-width="3"/>',
    text(54, 184, 'Быстрый старт', 13, fill='#1d4ed8', weight='600'),
    text(54, 222, 'Архитектура системы', 13, fill='#475569'),
    text(54, 258, 'Шаблоны и темы', 13, fill='#475569'),
    text(54, 294, 'Модули и плагины', 13, fill='#475569'),
    text(48, 340, 'СПРАВОЧНИК API', 11, fill='#94a3b8', weight='700'),
    text(54, 370, 'Аутентификация & CSRF', 13, fill='#475569'),
    text(54, 404, 'Структура контента', 13, fill='#475569'),
    text(54, 438, 'Маркетинг & Файлы', 13, fill='#475569'),
    # Sidebar footer
    line(48, 730, 208, '#e2e8f0'),
    text(48, 762, 'GitHub ↗      RSS', 12, fill='#64748b'),
    # Main Docs Content
    # Breadcrumbs
    text(330, 52, 'Документация  /  Начало работы  /  Быстрый старт', 12, fill='#64748b'),
    # Page Header
    text(330, 106, 'Быстрый старт с GoPress', 36, fill='#0f172a', weight='700'),
    text(330, 140, 'Пошаговое руководство по созданию вашего сайта и настройке локального окружения разработчика.', 15, fill='#475569'),
    # Info Callout Box
    rect(330, 168, 890, 68, '#eff6ff', 8, stroke='#bfdbfe'),
    line(330, 168, 0, '#2563eb'),
    '<path d="M330 168v68" stroke="#2563eb" stroke-width="4"/>',
    text(354, 194, 'Совет разработчикам', 13, fill='#1e40af', weight='700'),
    text(354, 218, 'Шаблоны не требуют внешних баз данных или сторонних репозиториев — SQLite встроен.', 13, fill='#1d4ed8'),
    # Section 1
    text(330, 276, '1. Установка и запуск сервера', 20, fill='#0f172a', weight='700'),
    # Code block
    rect(330, 296, 890, 160, '#0f172a', 8),
    rect(330, 296, 890, 34, '#1e293b', 8),
    rect(330, 320, 890, 10, '#1e293b'),
    text(350, 318, 'bash — терминал', 11, fill='#94a3b8', weight='600'),
    text(350, 360, '# Клонируйте репозиторий или скачайте бинарник', 13, fill='#64748b', font='monospace'),
    text(350, 388, '$ git clone https://github.com/gopress/gopress.git', 13, fill='#38bdf8', font='monospace'),
    text(350, 416, '$ cd gopress && make build', 13, fill='#e2e8f0', font='monospace'),
    text(350, 444, '$ ./gopress serve', 13, fill='#4ade80', font='monospace'),
    # Section 2
    text(330, 496, '2. Доступ к панели администратора', 20, fill='#0f172a', weight='700'),
    text(330, 526, 'Откройте в браузере http://localhost:8080/admin для перехода к первичной установке.', 15, fill='#475569'),
    # Bottom Pagination Cards
    rect(330, 580, 420, 64, '#f8fafc', 8, stroke='#e2e8f0'),
    text(354, 604, '← Назад', 11, fill='#64748b'),
    text(354, 626, 'Введение в архитектуру', 14, fill='#0f172a', weight='600'),
    rect(800, 580, 420, 64, '#f8fafc', 8, stroke='#e2e8f0'),
    text(824, 604, 'Далее →', 11, fill='#2563eb'),
    text(824, 626, 'Конфигурация SEO и модулей', 14, fill='#0f172a', weight='600'),
]
svg('minimal', p_docs)

# -------------------------------------------------------------
# 3. TEMPLATE 3: ПОРТФОЛИО (Studio - Dark Agency & Projects Showcase)
# -------------------------------------------------------------
logo_studio = f'<g transform="translate(54, 34) scale(0.055)" fill="#f1f0e9">{gopress_paths}</g>'
p_studio = [
    rect(0, 0, 1280, 800, '#0f1115'),
    logo_studio,
    text(820, 54, 'Проекты          О студии          Услуги          Контакты          Поиск ↗', 13, fill='#9ca3af', weight='500'),
    line(54, 86, 1172, '#22242a'),
    # Hero Section
    text(54, 128, '●  НЕЗАВИСИМОЕ ПРОСТРАНСТВО ИДЕЙ', 11, fill='#ffb86b', weight='700'),
    text(52, 230, 'Идеи. Люди.', 108, fill='#f1f0e9', weight='700'),
    text(52, 330, 'Результат.', 108, fill='#ffb86b', weight='700'),
    text(54, 385, 'Создаём цифровые продукты с характером и вниманием к каждой детали.', 16, fill='#9ca3af'),
    rect(1010, 350, 216, 48, '#ffb86b', 24),
    text(1040, 380, 'Смотреть проекты ↓', 14, fill='#0f1115', weight='600'),
    line(54, 426, 1172, '#22242a'),
    # Projects Header
    text(54, 464, 'Избранные проекты', 20, fill='#f1f0e9', weight='600'),
    text(1020, 462, 'Все проекты     Брендинг     Интерфейсы', 12, fill='#9ca3af'),
    # Project Cards (2 columns)
    # Card 1 (Left)
    rect(54, 490, 570, 200, '#1a1d24', 8),
    rect(54, 490, 570, 200, '#2a2d36', 8),
    f'<image x="54" y="490" width="570" height="200" preserveAspectRatio="xMidYMid slice" href="data:image/webp;base64,{photo}"/>',
    f'<circle cx="584" cy="650" r="20" fill="#ffb86b"/>',
    text(578, 656, '↗', 18, fill='#0f1115', weight='700'),
    text(54, 720, 'ДИЗАЙН · 2026', 10, fill='#ffb86b', weight='700'),
    text(54, 748, 'Тихий дом — архитектура и свет', 18, fill='#f1f0e9', weight='600'),
    text(54, 772, 'Исследование взаимодействия природных текстур и пространства.', 13, fill='#9ca3af'),
    # Card 2 (Right)
    rect(656, 490, 570, 200, '#1a1d24', 8),
    rect(656, 490, 570, 200, '#2a2d36', 8),
    f'<image x="656" y="490" width="570" height="200" preserveAspectRatio="xMidYMid slice" href="data:image/webp;base64,{photo}"/>',
    f'<circle cx="1186" cy="650" r="20" fill="#ffb86b"/>',
    text(1180, 656, '↗', 18, fill='#0f1115', weight='700'),
    text(656, 720, 'ПРАКТИКА · 2026', 10, fill='#ffb86b', weight='700'),
    text(656, 748, 'Новый способ видеть привычные вещи', 18, fill='#f1f0e9', weight='600'),
    text(656, 772, 'Концепция цифрового продукта для студии визуальных коммуникаций.', 13, fill='#9ca3af'),
]
svg('studio', p_studio)

print("Successfully built vector previews for journal (Блог), minimal (Документация), and studio (Портфолио)!")
