import { describe, expect, it } from 'vitest';

import {
  ANNOUNCEMENT_FORM_DEFAULT_VALUES,
  ANNOUNCEMENTS_PAGINATION_SCHEMA,
  announcementFormSchema,
  announcementsSearchSchema,
  type AnnouncementFormValues,
} from '../lib/announcements-schema';

function validOverrides(): Partial<AnnouncementFormValues> {
  return { title: 'Notice', message: 'Hello everyone' };
}

function validAnnouncementForm(): AnnouncementFormValues {
  return { ...ANNOUNCEMENT_FORM_DEFAULT_VALUES, ...validOverrides() };
}

// ---------------------------------------------------------------------------
// happy path
// ---------------------------------------------------------------------------

describe('announcementFormSchema — happy path', () => {
  it('parses a minimal valid form', () => {
    expect(announcementFormSchema.safeParse(validAnnouncementForm()).success).toBe(
      true,
    );
  });

  it('trims title and message before validating', () => {
    const result = announcementFormSchema.parse({
      ...validAnnouncementForm(),
      title: '  Notice  ',
      message: '  Hello  ',
    });
    expect(result.title).toBe('Notice');
    expect(result.message).toBe('Hello');
  });
});

// ---------------------------------------------------------------------------
// required fields
// ---------------------------------------------------------------------------

describe('announcementFormSchema — required fields', () => {
  it('rejects an empty title with titleRequired', () => {
    const result = announcementFormSchema.safeParse({
      ...validAnnouncementForm(),
      title: '',
    });
    expect(result.success).toBe(false);
    if (result.success) return;
    expect(result.error.issues[0]?.message).toBe(
      'siteAnnouncements.form.errors.titleRequired',
    );
  });

  it('rejects an empty message with messageRequired', () => {
    const result = announcementFormSchema.safeParse({
      ...validAnnouncementForm(),
      message: '',
    });
    expect(result.success).toBe(false);
    if (result.success) return;
    expect(result.error.issues[0]?.message).toBe(
      'siteAnnouncements.form.errors.messageRequired',
    );
  });
});

// ---------------------------------------------------------------------------
// length bounds
// ---------------------------------------------------------------------------

describe('announcementFormSchema — length bounds', () => {
  it('rejects a title over 200 chars', () => {
    const result = announcementFormSchema.safeParse({
      ...validAnnouncementForm(),
      title: 'x'.repeat(201),
    });
    expect(result.success).toBe(false);
    if (result.success) return;
    expect(result.error.issues[0]?.message).toBe(
      'siteAnnouncements.form.errors.titleTooLong',
    );
  });

  it('rejects a message over 5000 chars', () => {
    const result = announcementFormSchema.safeParse({
      ...validAnnouncementForm(),
      message: 'x'.repeat(5001),
    });
    expect(result.success).toBe(false);
    if (result.success) return;
    expect(result.error.issues[0]?.message).toBe(
      'siteAnnouncements.form.errors.messageTooLong',
    );
  });

  it('accepts exactly 200-char title and 5000-char message', () => {
    expect(
      announcementFormSchema.safeParse({
        ...validAnnouncementForm(),
        title: 'x'.repeat(200),
      }).success,
    ).toBe(true);
    expect(
      announcementFormSchema.safeParse({
        ...validAnnouncementForm(),
        message: 'x'.repeat(5000),
      }).success,
    ).toBe(true);
  });
});

// ---------------------------------------------------------------------------
// severity enum
// ---------------------------------------------------------------------------

describe('announcementFormSchema — severity enum', () => {
  it('accepts each documented severity', () => {
    for (const severity of ['info', 'warning', 'critical'] as const) {
      expect(
        announcementFormSchema.safeParse({ ...validAnnouncementForm(), severity })
          .success,
      ).toBe(true);
    }
  });

  it('rejects an unknown severity with an enum error', () => {
    const result = announcementFormSchema.safeParse({
      ...validAnnouncementForm(),
      severity: 'fatal' as AnnouncementFormValues['severity'],
    });
    expect(result.success).toBe(false);
    if (result.success) return;
    expect(result.error.issues[0]?.message).toContain('Invalid option');
  });
});

// ---------------------------------------------------------------------------
// link refine
// ---------------------------------------------------------------------------

describe('announcementFormSchema — link', () => {
  it('accepts an empty link', () => {
    expect(
      announcementFormSchema.safeParse({ ...validAnnouncementForm(), link: '' })
        .success,
    ).toBe(true);
  });

  it('accepts http / https links', () => {
    expect(
      announcementFormSchema.safeParse({
        ...validAnnouncementForm(),
        link: 'https://example.com',
      }).success,
    ).toBe(true);
  });

  it('rejects a non-http link with invalidLink', () => {
    const result = announcementFormSchema.safeParse({
      ...validAnnouncementForm(),
      link: 'javascript:alert(1)',
    });
    expect(result.success).toBe(false);
    if (result.success) return;
    expect(result.error.issues[0]?.message).toBe(
      'siteAnnouncements.form.errors.invalidLink',
    );
  });
});

// ---------------------------------------------------------------------------
// search schema + pagination
// ---------------------------------------------------------------------------

describe('announcementsSearchSchema', () => {
  it('accepts severity / enabled as plain strings (no enum coercion)', () => {
    expect(
      announcementsSearchSchema.parse({
        severity: 'whatever',
        enabled: 'true',
      }),
    ).toMatchObject({ severity: 'whatever', enabled: 'true' });
  });

  it('accepts page 0', () => {
    expect(announcementsSearchSchema.parse({ page: '0' }).page).toBe(0);
  });

  it('rejects pageSize above 200', () => {
    expect(
      announcementsSearchSchema.safeParse({ pageSize: '201' }).success,
    ).toBe(false);
  });
});

describe('ANNOUNCEMENTS_PAGINATION_SCHEMA', () => {
  it('applies defaults to an empty input', () => {
    expect(ANNOUNCEMENTS_PAGINATION_SCHEMA.parse({})).toEqual({
      pageIndex: 0,
      pageSize: 20,
    });
  });
});
