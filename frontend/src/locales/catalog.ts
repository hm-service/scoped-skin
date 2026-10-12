/**
 * 文案表的形状：每条都要中英两栏，源语言是简体中文。
 *
 * 键名规范（点号分层的小写符号名）与「用到但表里没有」这两件事由类型系统管：
 * t() 的参数类型就是合并后 catalog 的键，写错键名 / 漏配文案 vue-tsc 直接报错。
 */
export type Catalog = Record<string, { zh: string; en: string }>;
