# 🏃 WikiRacer - Быстрый поиск пути между статьями Wikipedia

Поиск кратчайшего пути между двумя статьями Wikipedia с использованием двунаправленного Greedy Best-First Search и мультиязыковых interwiki ссылок.

## 📊 Производительность

| Тест | Go | Python |
|------|-----|--------|
| Ибраево → Arch Linux | ~800мс | ~1.2с |
| Россия → Linux | ~600мс | ~800мс |
| Москва → Python | ~600мс | ~700мс |

## 🚀 Запуск Go решения

### Требования
- Go 1.21+

### Установка зависимостей
```bash
go mod download
```

### Сборка

**Linux / macOS:**
```bash
go build -o wiki main.go
```

**Windows:**
```powershell
go build -o wiki.exe main.go
```

### Запуск

**Linux / macOS:**
```bash
# По умолчанию: Ибраево → Arch Linux
./wiki

# Свои статьи (на русском)
./wiki "Москва" "Python"

# Свои статьи + язык
./wiki "Moscow" "Linux" en
```

**Windows:**
```powershell
# По умолчанию
.\wiki.exe

# Свои статьи
.\wiki.exe "Москва" "Python"

# Свои статьи + язык
.\wiki.exe "Moscow" "Linux" en
```

### Примеры
```bash
# Linux/macOS
./wiki "Россия" "SpaceX"
./wiki "Кошка" "Космос"

# Windows
.\wiki.exe "Россия" "SpaceX"
.\wiki.exe "Кошка" "Космос"
```

---

## 🐍 Запуск Python решения

### Требования
- Python 3.10+
- aiohttp

### Установка зависимостей

**Linux / macOS:**
```bash
python3 -m pip install aiohttp
# или с venv
python3 -m venv .venv
source .venv/bin/activate
pip install aiohttp
```

**Windows:**
```powershell
python -m pip install aiohttp
# или с venv
python -m venv .venv
.\.venv\Scripts\Activate.ps1
pip install aiohttp
```

### Запуск

**Linux / macOS:**
```bash
# По умолчанию: Ибраево → Arch Linux
python3 main.py

# Свои статьи
python3 main.py "Москва" "Python"
```

**Windows:**
```powershell
# По умолчанию
python main.py

# Свои статьи
python main.py "Москва" "Python"
```

### Примеры
```bash
# Linux/macOS
python3 main.py "Россия" "Linux"
python3 main.py "Кошка" "Космос"

# Windows
python main.py "Россия" "Linux"
python main.py "Кошка" "Космос"
```

---

## 🧠 Алгоритм

**Bidirectional Greedy Best-First Search:**

1. **Двунаправленный поиск** - одновременно от старта к цели и от цели к старту
2. **Priority Queue** - узлы сортируются по эвристике близости к цели
3. **Interwiki мосты** - используем ссылки между языковыми версиями Wikipedia
4. **8 языков параллельно** - en, ru, de, fr, es, it, pt, uk

### Эвристика
- Бонус за совпадение языка с целью
- Бонус за общие слова в названии
- Бонус за английский (больше связей)
- Штраф за длинные названия

---

## 📁 Структура проекта

```
sirius_kurci/
├── main.go      # Go решение (быстрее)
├── main.py      # Python решение
├── go.mod       # Go модули
└── README.md    # Документация
```

---

## ⚡ Оптимизации

### Go
- HTTP/2 multiplexing
- Connection pooling (1000 idle connections)
- Параллельные goroutines
- sync.Map для concurrent доступа

### Python
- asyncio + aiohttp
- Unlimited connections
- True pipeline с FIRST_COMPLETED
- Lock-protected shared state

---

## 🔧 Конфигурация

### Таймауты
- Go: 1.5s per request, 5s total
- Python: 2s connect, 5s total

### Языки Wikipedia
Оба решения используют 8 языковых эндпоинтов:
- English (en) - основной хаб
- Russian (ru)
- German (de)
- French (fr)
- Spanish (es)
- Italian (it)
- Portuguese (pt)
- Ukrainian (uk)

---

## 📈 Бенчмарки

Запуск 5 раз подряд:

**Linux / macOS:**
```bash
# Go
for i in {1..5}; do ./wiki; done

# Python
for i in {1..5}; do python3 main.py; done
```

**Windows (PowerShell):**
```powershell
# Go
1..5 | ForEach-Object { .\wiki.exe }

# Python
1..5 | ForEach-Object { python main.py }
```

Типичные результаты:
- **Go**: 600-1000мс, 5-10 API запросов
- **Python**: 800-1500мс, 10-15 API запросов
