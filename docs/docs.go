// Package docs WikiRacer API.
//
// API для поиска кратчайшего пути между статьями Wikipedia
//
//	Schemes: http
//	Host: localhost:3000
//	BasePath: /api/v1
//	Version: 1.0.0
//
//	Consumes:
//	- application/json
//
//	Produces:
//	- application/json
//
// swagger:meta
package docs

import "github.com/swaggo/swag"

const docTemplate = `{
    "swagger": "2.0",
    "info": {
        "description": "API для поиска кратчайшего пути между статьями Wikipedia. Использует bidirectional Greedy Best-First Search с поддержкой 8 языков.",
        "title": "WikiRacer API",
        "termsOfService": "http://swagger.io/terms/",
        "contact": {
            "name": "API Support",
            "email": "support@wikiracer.local"
        },
        "license": {
            "name": "MIT",
            "url": "https://opensource.org/licenses/MIT"
        },
        "version": "1.0.0"
    },
    "host": "localhost:3000",
    "basePath": "/api/v1",
    "paths": {
        "/health": {
            "get": {
                "description": "Возвращает статус API",
                "produces": ["application/json"],
                "tags": ["health"],
                "summary": "Проверка состояния API",
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "type": "object",
                            "properties": {
                                "status": {"type": "string", "example": "ok"},
                                "service": {"type": "string", "example": "WikiRacer API"},
                                "version": {"type": "string", "example": "1.0.0"}
                            }
                        }
                    }
                }
            }
        },
        "/search": {
            "get": {
                "description": "Ищет кратчайший путь между двумя статьями Wikipedia используя bidirectional search",
                "produces": ["application/json"],
                "tags": ["search"],
                "summary": "Найти путь между статьями Wikipedia (GET)",
                "parameters": [
                    {
                        "type": "string",
                        "description": "Начальная статья",
                        "name": "from",
                        "in": "query",
                        "required": true,
                        "example": "Кошка"
                    },
                    {
                        "type": "string",
                        "description": "Конечная статья",
                        "name": "to",
                        "in": "query",
                        "required": true,
                        "example": "Теория относительности"
                    },
                    {
                        "type": "string",
                        "description": "Язык по умолчанию (ru, en, de, fr, es, it, pt, uk)",
                        "name": "lang",
                        "in": "query",
                        "default": "ru",
                        "example": "ru"
                    }
                ],
                "responses": {
                    "200": {
                        "description": "Успешный поиск",
                        "schema": {"$ref": "#/definitions/SearchResponse"}
                    },
                    "400": {
                        "description": "Ошибка в параметрах",
                        "schema": {"$ref": "#/definitions/ErrorResponse"}
                    },
                    "404": {
                        "description": "Путь не найден",
                        "schema": {"$ref": "#/definitions/ErrorResponse"}
                    }
                }
            },
            "post": {
                "description": "Ищет кратчайший путь между двумя статьями Wikipedia используя bidirectional search",
                "consumes": ["application/json"],
                "produces": ["application/json"],
                "tags": ["search"],
                "summary": "Найти путь между статьями Wikipedia (POST)",
                "parameters": [
                    {
                        "description": "Параметры поиска",
                        "name": "request",
                        "in": "body",
                        "required": true,
                        "schema": {"$ref": "#/definitions/SearchRequest"}
                    }
                ],
                "responses": {
                    "200": {
                        "description": "Успешный поиск",
                        "schema": {"$ref": "#/definitions/SearchResponse"}
                    },
                    "400": {
                        "description": "Ошибка в параметрах",
                        "schema": {"$ref": "#/definitions/ErrorResponse"}
                    },
                    "404": {
                        "description": "Путь не найден",
                        "schema": {"$ref": "#/definitions/ErrorResponse"}
                    }
                }
            }
        }
    },
    "definitions": {
        "SearchRequest": {
            "type": "object",
            "required": ["from", "to"],
            "properties": {
                "from": {
                    "type": "string",
                    "description": "Начальная статья",
                    "example": "Кошка"
                },
                "to": {
                    "type": "string",
                    "description": "Конечная статья",
                    "example": "Теория относительности"
                },
                "lang": {
                    "type": "string",
                    "description": "Язык по умолчанию",
                    "default": "ru",
                    "example": "ru"
                }
            }
        },
        "SearchResponse": {
            "type": "object",
            "properties": {
                "success": {
                    "type": "boolean",
                    "example": true
                },
                "from": {
                    "type": "string",
                    "example": "Кошка"
                },
                "to": {
                    "type": "string",
                    "example": "Теория относительности"
                },
                "path_length": {
                    "type": "integer",
                    "example": 3
                },
                "path": {
                    "type": "array",
                    "items": {"$ref": "#/definitions/PathStep"}
                },
                "transitions": {
                    "type": "array",
                    "items": {"$ref": "#/definitions/Transition"}
                },
                "stats": {"$ref": "#/definitions/SearchStats"}
            }
        },
        "PathStep": {
            "type": "object",
            "properties": {
                "step": {
                    "type": "integer",
                    "description": "Номер шага (1-based)",
                    "example": 1
                },
                "title": {
                    "type": "string",
                    "description": "Название статьи",
                    "example": "Кошка"
                },
                "lang": {
                    "type": "string",
                    "description": "Язык статьи",
                    "example": "ru"
                },
                "url": {
                    "type": "string",
                    "description": "URL статьи в Wikipedia",
                    "example": "https://ru.wikipedia.org/wiki/Кошка"
                },
                "full_name": {
                    "type": "string",
                    "description": "Полное имя (lang:title)",
                    "example": "ru:Кошка"
                }
            }
        },
        "Transition": {
            "type": "object",
            "properties": {
                "from": {
                    "type": "string",
                    "description": "Исходная статья",
                    "example": "Кошка"
                },
                "to": {
                    "type": "string",
                    "description": "Целевая статья",
                    "example": "Квантовая механика"
                },
                "type": {
                    "type": "string",
                    "description": "Тип перехода (link или interwiki)",
                    "enum": ["link", "interwiki"],
                    "example": "link"
                },
                "description": {
                    "type": "string",
                    "description": "Описание как найти ссылку",
                    "example": "Найти 'Квантовая механика' в статье 'Кошка'"
                },
                "check_url": {
                    "type": "string",
                    "description": "URL для проверки перехода",
                    "example": "https://ru.wikipedia.org/wiki/Кошка"
                }
            }
        },
        "SearchStats": {
            "type": "object",
            "properties": {
                "duration": {
                    "type": "string",
                    "description": "Время поиска (human readable)",
                    "example": "823.45ms"
                },
                "duration_ms": {
                    "type": "number",
                    "description": "Время поиска в миллисекундах",
                    "example": 823.45
                },
                "request_count": {
                    "type": "integer",
                    "description": "Количество запросов к Wikipedia API",
                    "example": 12
                }
            }
        },
        "ErrorResponse": {
            "type": "object",
            "properties": {
                "success": {
                    "type": "boolean",
                    "example": false
                },
                "error": {
                    "type": "string",
                    "description": "Описание ошибки",
                    "example": "Путь не найден"
                },
                "code": {
                    "type": "string",
                    "description": "Код ошибки",
                    "enum": ["INVALID_REQUEST", "MISSING_PARAMS", "PATH_NOT_FOUND"],
                    "example": "PATH_NOT_FOUND"
                }
            }
        }
    }
}`

// SwaggerInfo holds exported Swagger Info so clients can modify it
var SwaggerInfo = &swag.Spec{
	Version:          "1.0.0",
	Host:             "localhost:3000",
	BasePath:         "/api/v1",
	Schemes:          []string{},
	Title:            "WikiRacer API",
	Description:      "API для поиска кратчайшего пути между статьями Wikipedia",
	InfoInstanceName: "swagger",
	SwaggerTemplate:  docTemplate,
	LeftDelim:        "{{",
	RightDelim:       "}}",
}

func init() {
	swag.Register(SwaggerInfo.InstanceName(), SwaggerInfo)
}
