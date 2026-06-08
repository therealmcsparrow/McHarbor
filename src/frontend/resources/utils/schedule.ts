// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

export type ScheduleUnit = 'hour' | 'day' | 'week' | 'month' | 'year';

export type ScheduleT = (key: string, options?: Record<string, unknown>) => string;

function pad(value: number) {
  return String(value).padStart(2, '0');
}

export function localDateTimeValue(date = new Date()) {
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

export function cronFromReadableSchedule(interval: number, unit: ScheduleUnit, startsAtValue: string) {
  const date = startsAtValue ? new Date(startsAtValue) : new Date();
  const minute = Number.isFinite(date.getMinutes()) ? date.getMinutes() : 0;
  const hour = Number.isFinite(date.getHours()) ? date.getHours() : 0;
  const day = Number.isFinite(date.getDate()) ? date.getDate() : 1;
  const month = Number.isFinite(date.getMonth()) ? date.getMonth() + 1 : 1;
  const weekday = Number.isFinite(date.getDay()) ? date.getDay() : 0;
  const every = Math.max(1, Math.floor(interval || 1));

  switch (unit) {
    case 'hour':
      return every === 1 ? `${minute} * * * *` : `${minute} */${Math.min(every, 23)} * * *`;
    case 'day':
      return every === 1 ? `${minute} ${hour} * * *` : `${minute} ${hour} */${Math.min(every, 31)} * *`;
    case 'week':
      return every === 1 ? `${minute} ${hour} * * ${weekday}` : `${minute} ${hour} */${Math.min(every * 7, 31)} * *`;
    case 'month':
      return every === 1 ? `${minute} ${hour} ${day} * *` : `${minute} ${hour} ${day} */${Math.min(every, 12)} *`;
    case 'year':
      return `${minute} ${hour} ${day} ${month} *`;
  }
}

export function describeCron(expression: string | undefined, t: ScheduleT) {
  const fields = (expression ?? '').trim().split(/\s+/);
  if (fields.length !== 5) return expression ?? '';

  const [minute, hour, day, month, weekday] = fields as [string, string, string, string, string];
  if (minute === '*' && hour === '*' && day === '*' && month === '*' && weekday === '*') {
    return t('schedule.readable.everyMinute');
  }
  if (/^\d+$/.test(minute) && hour === '*' && day === '*' && month === '*' && weekday === '*') {
    return t('schedule.readable.everyHourAt', { minute: pad(Number(minute)) });
  }
  if (/^\d+$/.test(minute) && /^\/?\d+$/.test(hour.replace('*', '')) && hour.startsWith('*/') && day === '*' && month === '*' && weekday === '*') {
    return t('schedule.readable.everyNHoursAt', { count: hour.slice(2), minute: pad(Number(minute)) });
  }
  if (/^\d+$/.test(minute) && /^\d+$/.test(hour) && day === '*' && month === '*' && weekday === '*') {
    return t('schedule.readable.everyDayAt', { time: `${pad(Number(hour))}:${pad(Number(minute))}` });
  }
  if (/^\d+$/.test(minute) && /^\d+$/.test(hour) && day.startsWith('*/') && month === '*' && weekday === '*') {
    return t('schedule.readable.everyNDaysAt', { count: day.slice(2), time: `${pad(Number(hour))}:${pad(Number(minute))}` });
  }
  if (/^\d+$/.test(minute) && /^\d+$/.test(hour) && day === '*' && month === '*' && /^\d+$/.test(weekday)) {
    return t('schedule.readable.everyWeekdayAt', {
      weekday: t(`schedule.weekdays.${Number(weekday) % 7}`),
      time: `${pad(Number(hour))}:${pad(Number(minute))}`,
    });
  }
  if (/^\d+$/.test(minute) && /^\d+$/.test(hour) && /^\d+$/.test(day) && month === '*' && weekday === '*') {
    return t('schedule.readable.everyMonthAt', { day, time: `${pad(Number(hour))}:${pad(Number(minute))}` });
  }
  if (/^\d+$/.test(minute) && /^\d+$/.test(hour) && /^\d+$/.test(day) && /^\d+$/.test(month) && weekday === '*') {
    return t('schedule.readable.everyYearAt', { month, day, time: `${pad(Number(hour))}:${pad(Number(minute))}` });
  }
  return expression ?? '';
}
