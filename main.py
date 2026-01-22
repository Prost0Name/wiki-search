import asyncio
import aiohttp
import time
from collections import defaultdict
from dataclasses import dataclass, field
from typing import Dict, Set, List, Optional, Tuple
import heapq

# Мультиязыковые эндпоинты
WIKI_APIS = {
    "en": "https://en.wikipedia.org/w/api.php",
    "ru": "https://ru.wikipedia.org/w/api.php",
    "de": "https://de.wikipedia.org/w/api.php",
    "fr": "https://fr.wikipedia.org/w/api.php",
    "es": "https://es.wikipedia.org/w/api.php",
    "it": "https://it.wikipedia.org/w/api.php",
    "pt": "https://pt.wikipedia.org/w/api.php",
    "ja": "https://ja.wikipedia.org/w/api.php",
    "zh": "https://zh.wikipedia.org/w/api.php",
    "pl": "https://pl.wikipedia.org/w/api.php",
    "nl": "https://nl.wikipedia.org/w/api.php",
    "uk": "https://uk.wikipedia.org/w/api.php",
}

HEADERS = {"User-Agent": "WikiRacer/4.0 (speedrun@wiki.net)"}


class PathFound(Exception):
    """Исключение для мгновенной остановки"""
    def __init__(self, meeting_node, parent_node, direction):
        self.meeting_node = meeting_node
        self.parent_node = parent_node
        self.direction = direction


@dataclass(frozen=True)
class WikiNode:
    """Узел графа"""
    title: str
    lang: str
    
    def __hash__(self):
        return hash((self.title.lower(), self.lang))
    
    def __eq__(self, other):
        if not isinstance(other, WikiNode):
            return False
        return self.title.lower() == other.title.lower() and self.lang == other.lang
    
    def __repr__(self):
        return f"{self.lang}:{self.title}"


class UltraFastWikiSearcher:
    """
    Агрессивный pipeline-поиск с максимальным насыщением сети
    """
    
    def __init__(self):
        self.visited_forward: Dict[WikiNode, Optional[WikiNode]] = {}
        self.visited_backward: Dict[WikiNode, Optional[WikiNode]] = {}
        self.visited_forward_set: Set[WikiNode] = set()
        self.visited_backward_set: Set[WikiNode] = set()
        
        self.pending_forward: List[Tuple[WikiNode, int]] = []
        self.pending_backward: List[Tuple[WikiNode, int]] = []
        
        self.found = False
        self.result_path: Optional[List[str]] = None
        self.requests_count = 0
        self.lock = asyncio.Lock()
        
    async def fetch_links(
        self, 
        session: aiohttp.ClientSession,
        titles: List[str], 
        lang: str,
        direction: str,
        depth: int
    ):
        """Получает ссылки и сразу добавляет новые узлы"""
        if self.found:
            return
            
        api_url = WIKI_APIS.get(lang, WIKI_APIS["en"])
        
        params = {
            "action": "query",
            "format": "json",
            "prop": "links|langlinks",
            "titles": "|".join(titles[:50]),
            "pllimit": "max",
            "lllimit": "max",
            "plnamespace": 0,
            "redirects": 1,
        }
        
        try:
            async with session.get(api_url, params=params, headers=HEADERS) as resp:
                self.requests_count += 1
                data = await resp.json()
                
                if self.found:
                    return
                
                pages = data.get("query", {}).get("pages", {})
                
                async with self.lock:
                    for page_data in pages.values():
                        if self.found:
                            return
                            
                        parent_title = page_data.get("title")
                        if not parent_title:
                            continue
                        
                        parent_node = WikiNode(parent_title, lang)
                        
                        if direction == "forward":
                            visited_own = self.visited_forward_set
                            visited_other = self.visited_backward_set
                            visited_dict = self.visited_forward
                            pending = self.pending_forward
                        else:
                            visited_own = self.visited_backward_set
                            visited_other = self.visited_forward_set
                            visited_dict = self.visited_backward
                            pending = self.pending_backward
                        
                        # Обрабатываем ссылки
                        for link in page_data.get("links", []):
                            child = WikiNode(link["title"], lang)
                            
                            if child in visited_other:
                                self.found = True
                                visited_dict[child] = parent_node
                                visited_own.add(child)
                                self.result_path = self._construct_path(child)
                                return
                            
                            if child not in visited_own:
                                visited_dict[child] = parent_node
                                visited_own.add(child)
                                pending.append((child, depth + 1))
                        
                        # Обрабатываем interwiki
                        for ll in page_data.get("langlinks", []):
                            ll_lang = ll.get("lang")
                            ll_title = ll.get("*")
                            if ll_lang and ll_title and ll_lang in WIKI_APIS:
                                child = WikiNode(ll_title, ll_lang)
                                
                                if child in visited_other:
                                    self.found = True
                                    visited_dict[child] = parent_node
                                    visited_own.add(child)
                                    self.result_path = self._construct_path(child)
                                    return
                                
                                if child not in visited_own:
                                    visited_dict[child] = parent_node
                                    visited_own.add(child)
                                    pending.append((child, depth))
                                    
        except:
            pass
    
    def _construct_path(self, meeting_point: WikiNode) -> List[str]:
        """Восстанавливает путь"""
        path_start = []
        curr = meeting_point
        while curr is not None:
            path_start.append(str(curr))
            curr = self.visited_forward.get(curr)
        path_start.reverse()
        
        path_end = []
        curr = self.visited_backward.get(meeting_point)
        while curr is not None:
            path_end.append(str(curr))
            curr = self.visited_backward.get(curr)
        
        return path_start + path_end
    
    async def resolve_langlinks(self, session: aiohttp.ClientSession, title: str, source_lang: str) -> Dict[str, WikiNode]:
        """Получает все языковые версии"""
        result = {source_lang: WikiNode(title, source_lang)}
        api_url = WIKI_APIS.get(source_lang, WIKI_APIS["ru"])
        
        params = {
            "action": "query",
            "format": "json",
            "prop": "langlinks",
            "titles": title,
            "lllimit": "max",
            "redirects": 1,
        }
        
        try:
            async with session.get(api_url, params=params, headers=HEADERS) as resp:
                self.requests_count += 1
                data = await resp.json()
                pages = data.get("query", {}).get("pages", {})
                
                for page_data in pages.values():
                    for ll in page_data.get("langlinks", []):
                        ll_lang = ll.get("lang")
                        ll_title = ll.get("*")
                        if ll_lang and ll_title and ll_lang in WIKI_APIS:
                            result[ll_lang] = WikiNode(ll_title, ll_lang)
        except:
            pass
        
        return result
    
    async def search(self, start: str, end: str, source_lang: str = "ru") -> Optional[List[str]]:
        """Главный метод поиска"""
        
        # Агрессивные настройки - БЕЗ ЛИМИТОВ
        connector = aiohttp.TCPConnector(
            limit=0,  # Без лимита соединений!
            ttl_dns_cache=600,
            keepalive_timeout=60,
            enable_cleanup_closed=True,
            force_close=False,
        )
        
        # Очень короткие таймауты
        timeout = aiohttp.ClientTimeout(total=2, connect=0.5, sock_read=1.5)
        
        async with aiohttp.ClientSession(connector=connector, timeout=timeout) as session:
            print(f"🔍 Ищу путь: {start} → {end}")
            print(f"📡 Языки: {', '.join(WIKI_APIS.keys())}")
            
            # Получаем языковые версии параллельно
            start_langs, end_langs = await asyncio.gather(
                self.resolve_langlinks(session, start, source_lang),
                self.resolve_langlinks(session, end, source_lang)
            )
            
            print(f"📖 Start: {len(start_langs)} wiki | End: {len(end_langs)} wiki")
            
            # Инициализация
            for lang, node in start_langs.items():
                self.visited_forward[node] = None
                self.visited_forward_set.add(node)
                self.pending_forward.append((node, 0))
            
            for lang, node in end_langs.items():
                self.visited_backward[node] = None
                self.visited_backward_set.add(node)
                self.pending_backward.append((node, 0))
            
            # Прямое пересечение?
            intersection = self.visited_forward_set & self.visited_backward_set
            if intersection:
                meeting = intersection.pop()
                print(f"✅ Прямое совпадение!")
                return self._construct_path(meeting)
            
            # TRUE PIPELINE: запускаем всё сразу и обрабатываем по мере завершения
            all_tasks: Set[asyncio.Task] = set()
            iteration = 0
            
            while not self.found:
                iteration += 1
                
                # Создаём новые задачи из pending
                async with self.lock:
                    forward_by_lang: Dict[str, List[Tuple[str, int]]] = defaultdict(list)
                    backward_by_lang: Dict[str, List[Tuple[str, int]]] = defaultdict(list)
                    
                    for node, depth in self.pending_forward:
                        forward_by_lang[node.lang].append((node.title, depth))
                    self.pending_forward = []
                    
                    for node, depth in self.pending_backward:
                        backward_by_lang[node.lang].append((node.title, depth))
                    self.pending_backward = []
                
                new_tasks = []
                
                for lang, items in forward_by_lang.items():
                    for i in range(0, len(items), 50):
                        batch = items[i:i+50]
                        titles = [t for t, d in batch]
                        depth = batch[0][1]
                        task = asyncio.create_task(self.fetch_links(session, titles, lang, "forward", depth))
                        new_tasks.append(task)
                
                for lang, items in backward_by_lang.items():
                    for i in range(0, len(items), 50):
                        batch = items[i:i+50]
                        titles = [t for t, d in batch]
                        depth = batch[0][1]
                        task = asyncio.create_task(self.fetch_links(session, titles, lang, "backward", depth))
                        new_tasks.append(task)
                
                if not new_tasks and not all_tasks:
                    break
                
                all_tasks.update(new_tasks)
                
                if new_tasks:
                    print(f"⚡ +{len(new_tasks)} req | Active: {len(all_tasks)} | "
                          f"F:{len(self.visited_forward)} B:{len(self.visited_backward)}")
                
                if not all_tasks:
                    break
                
                # Ждём ЛЮБУЮ завершённую задачу (не все!)
                done, all_tasks = await asyncio.wait(all_tasks, timeout=0.1, return_when=asyncio.FIRST_COMPLETED)
                
                if self.found:
                    # Отменяем все оставшиеся
                    for task in all_tasks:
                        task.cancel()
                    print(f"✅ Путь найден!")
                    return self.result_path
                
                # Проверка пересечения
                async with self.lock:
                    intersection = self.visited_forward_set & self.visited_backward_set
                    if intersection:
                        for task in all_tasks:
                            task.cancel()
                        meeting = intersection.pop()
                        print(f"✅ Пересечение найдено!")
                        return self._construct_path(meeting)
            
            print(f"❌ Не найден")
            return None


async def main():
    import sys
    
    if len(sys.argv) >= 3:
        start_article = sys.argv[1]
        end_article = sys.argv[2]
        source_lang = sys.argv[3] if len(sys.argv) >= 4 else "ru"
    else:
        start_article = "Ибраево"
        end_article = "Arch Linux"
        source_lang = "ru"
        print(f"💡 Использование: python main.py <start> <end> [lang]\n")
    
    start_time = time.time()
    searcher = UltraFastWikiSearcher()
    
    try:
        path = await searcher.search(start_article, end_article, source_lang=source_lang)
        
        elapsed = time.time() - start_time
        print(f"\n⏱️ Время: {elapsed:.2f} сек")
        print(f"📊 Запросов: {searcher.requests_count}")
        
        if path:
            print(f"\n🎯 Путь ({len(path)} шагов):")
            for i, step in enumerate(path):
                print(f"  {i+1}. {step}")
            print(f"\n📍 {' → '.join(path)}")
        else:
            print("\n❌ Путь не найден")
            
    except KeyboardInterrupt:
        print("\n⛔ Остановлено")
    except Exception as e:
        print(f"\n💥 Ошибка: {e}")
        import traceback
        traceback.print_exc()


if __name__ == "__main__":
    asyncio.run(main())
