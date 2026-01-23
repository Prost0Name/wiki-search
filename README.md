# 🏃 WikiRacer - Быстрый поиск пути между статьями Wikipedia

Поиск кратчайшего пути между двумя статьями Wikipedia с использованием двунаправленного Greedy Best-First Search и мультиязыковых interwiki ссылок.

## 📊 Производительность

| Тест | API | CLI |
|------|-----|-----|
| Россия → Германия | ~714мс | ~800мс |
| Кошка → Теория относительности | ~800мс | ~900мс |
| Segment tree → Малое Ибраево | ~2.5с | ~3.5с |

## ✨ Три решения

### 🌐 `api.go` - REST API на Fiber v2
- **REST API** с Swagger UI документацией
- GET/POST эндпоинты
- JSON ответ с путём, ссылками и временем
- http://localhost:3000/swagger/index.html

### 🚀 `main.go` - CLI Optimized версия
- Агрессивные оптимизации (maxPerRound=250, timeout=800мс)
- Быстрое определение языка по символам
- Усиленная эвристика

### 📦 `simple.go` - CLI Simple версия  
- Базовая реализация (maxPerRound=100, timeout=1500мс)
- Стабильнее, но медленнее

## 🌐 REST API

### Запуск API сервера

```bash
go build -o wikiracer-api api.go
./wikiracer-api  # Запуск на http://localhost:3000
```

### Swagger UI

Открыть http://localhost:3000/swagger/index.html для интерактивной документации.

### Эндпоинты

#### GET /api/v1/search

```bash
curl "http://localhost:3000/api/v1/search?from=Кошка&to=Собака"
```

#### POST /api/v1/search

```bash
curl -X POST http://localhost:3000/api/v1/search \
  -H "Content-Type: application/json" \
  -d '{"from": "Кошка", "to": "Собака", "lang": "ru"}'
```

### Пример ответа

```json
{
  "success": true,
  "from": "Россия",
  "to": "Германия",
  "path_length": 2,
  "path": [
    {
      "step": 1,
      "title": "Россия",
      "lang": "ru",
      "url": "https://ru.wikipedia.org/wiki/Россия",
      "full_name": "ru:Россия"
    },
    {
      "step": 2,
      "title": "Германия",
      "lang": "ru", 
      "url": "https://ru.wikipedia.org/wiki/Германия",
      "full_name": "ru:Германия"
    }
  ],
  "transitions": [
    {
      "from": "Россия",
      "to": "Германия",
      "type": "link",
      "description": "Найти 'Германия' в статье 'Россия'",
      "check_url": "https://ru.wikipedia.org/wiki/Россия"
    }
  ],
  "stats": {
    "duration": "714.744ms",
    "duration_ms": 714.744,
    "request_count": 2
  }
}
```

## 🚀 CLI - Быстрый старт

### Требования
- Go 1.21+

### Установка

```bash
git clone <repo-url>
cd sirius_kurci
go mod download
```

### Сборка

```bash
# Optimized версия
go build -o wikiracer main.go        # Linux/macOS
go build -o wikiracer.exe main.go    # Windows

# Simple версия
go build -o wikisimple simple.go     # Linux/macOS
go build -o wikisimple.exe simple.go # Windows
```

### Запуск

```bash
# Optimized
./wikiracer "Кошка" "Квантовая механика"
.\wikiracer.exe "Кошка" "Квантовая механика"

# Simple
./wikisimple "Кошка" "Квантовая механика"
.\wikisimple.exe "Кошка" "Квантовая механика"
```

## 📖 Использование

```bash
# Базовое использование
./wikiracer "Начальная статья" "Конечная статья"

# Автоопределение языка (работает для любых языков)
./wikiracer "Segment tree" "Малое Ибраево"
./wikiracer "Pizza" "Квантовая механика"
./wikiracer "Eiffel Tower" "Москва"

# Явное указание языка (опционально)
./wikiracer "Moscow" "Linux" en
```

## 🔧 Примеры

### Linux / macOS
```bash
./wikiracer "Россия" "SpaceX"
./wikiracer "Кошка" "Космос"
./wikiracer "Python" "Математика"
./wikiracer "Apple" "Microsoft"
```

### Windows (PowerShell)
```powershell
.\wikiracer.exe "Россия" "SpaceX"
.\wikiracer.exe "Кошка" "Космос"
.\wikiracer.exe "Python" "Математика"
.\wikiracer.exe "Apple" "Microsoft"
```

## 🏗️ Архитектура

```
┌─────────────────────────────────────────────────────────┐
│                      WikiRacer                          │
├─────────────────────────────────────────────────────────┤
│  Bidirectional Greedy Best-First Search                 │
│  ┌─────────────┐                    ┌─────────────┐     │
│  │   Forward   │ ←── встреча ───→  │  Backward   │     │
│  │  (links)    │                    │ (linkshere) │     │
│  └─────────────┘                    └─────────────┘     │
├─────────────────────────────────────────────────────────┤
│  Мультиязычный поиск: ru, en, de, fr, es, it, pt, uk    │
├─────────────────────────────────────────────────────────┤
│  HTTP/2 + Параллельные запросы + Priority Queue         │
└─────────────────────────────────────────────────────────┘
```

## 📁 Структура проекта

```
sirius_kurci/
├── main.go          # Optimized решение
├── simple.go        # Simple решение
├── go.mod           # Go модуль
├── go.sum           # Зависимости
├── README.md        # Документация
└── .gitignore       # Игнорируемые файлы
```

## 🔬 Как это работает

1. **Автоопределение языка** - по символам (кириллица → ru, латиница → en)
2. **Forward поиск** - от стартовой статьи по исходящим ссылкам (`prop=links`)
3. **Backward поиск** - от конечной статьи по входящим ссылкам (`prop=linkshere`)
4. **Эвристика** - приоритет статьям с общими словами с целью
5. **Interwiki мосты** - переход между языковыми версиями
6. **Встреча** - когда forward и backward находят общую статью

## 📈 Сравнение версий

| Параметр | Optimized (`main.go`) | Simple (`simple.go`) |
|----------|----------------------|---------------------|
| Global timeout | 10с | 5с |
| Request timeout | 800мс | 1500мс |
| maxPerRound | 250 | 100 |
| detectLang | 2 языка (быстро) | 8 языков (полно) |
| Эвристика | Усиленная | Базовая |

## 📝 Лицензия

MIT
