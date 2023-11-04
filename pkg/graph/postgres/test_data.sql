/*
 * Copyright (c) 2023. Raft, LLC
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <https://www.gnu.org/licenses/>.
 */

drop schema if exists syncd cascade;
create schema syncd;
create table syncd.artists
(
    id      varchar not null primary key,
    name    varchar not null,
    version text    not null
);
create index artists__id_version_index on syncd.artists (id, version);

create table syncd.performers
(
    id      varchar not null primary key,
    name    varchar not null,
    label   varchar not null,
    version text    not null
);
create index performers__id_version on syncd.performers (id, version);
create index performers__label on syncd.performers (label);

create table syncd.performer_artists
(
    performer varchar not null references syncd.performers,
    artist    varchar not null
);
create unique index performer_artists__performer_artist on syncd.performer_artists (performer, artist);
create index performer_artists__artist on syncd.performer_artists (artist);

create table syncd.songs
(
    id        varchar not null primary key,
    name      varchar not null,
    composer  varchar not null,
    publisher varchar not null,
    version   text    not null
);
create index songs__id_version on syncd.songs (id, version);
create index songs__composer on syncd.songs (composer);
create index songs__publisher on syncd.songs (publisher);

create table syncd.performances
(
    performer varchar not null references syncd.performers (id),
    sequence  int     not null,
    song      varchar not null
);
create unique index performances__performer_sequence on syncd.performances (performer, sequence);
create index performances_song on syncd.performances (song);

insert into syncd.artists (id, name, version)
values ('8b9835e6-9928-4f6a-afba-ea3e80cf89d0', 'John Lennon', '1'),
       ('253cedce-05ed-44bc-a49d-6f570a18b2ef', 'Paul McCartney', '1'),
       ('57ecbe75-ba4c-4094-acca-d066b6c6d058', 'Ringo Starr', '1'),
       ('88a6845b-e286-49a0-aa09-d89309d12d33', 'George Harrison', '1');

insert into syncd.songs (id, name, composer, publisher, version)
values ('ec7f7744-1f8f-4086-bff6-4809ecb948f4', 'I Want to Hold Your Hand', '8b9835e6-9928-4f6a-afba-ea3e80cf89d0', 'VeeJay', '1'),
       ('3ce42a81-ae99-42b6-b4ed-d0dc1107e651', 'Please Please Me', '8b9835e6-9928-4f6a-afba-ea3e80cf89d0', 'VeeJay', '1');

insert into syncd.performers (id, name, label, version)
values ('16551b42-292f-4316-800c-880c864fcbfd', 'The Beatles', 'VeeJay', '1'),
       ('77bd209c-748d-48ea-9f1b-9fd6a3298f47', 'Paul McCartney', 'Apple Records', 1);

insert into syncd.performer_artists (performer, artist)
values ('16551b42-292f-4316-800c-880c864fcbfd', '8b9835e6-9928-4f6a-afba-ea3e80cf89d0'),
       ('16551b42-292f-4316-800c-880c864fcbfd', '253cedce-05ed-44bc-a49d-6f570a18b2ef'),
       ('16551b42-292f-4316-800c-880c864fcbfd', '57ecbe75-ba4c-4094-acca-d066b6c6d058'),
       ('16551b42-292f-4316-800c-880c864fcbfd', '88a6845b-e286-49a0-aa09-d89309d12d33'),
       ('77bd209c-748d-48ea-9f1b-9fd6a3298f47', '253cedce-05ed-44bc-a49d-6f570a18b2ef');

insert into syncd.performances (performer, sequence, song)
values ('16551b42-292f-4316-800c-880c864fcbfd', 1, 'ec7f7744-1f8f-4086-bff6-4809ecb948f4'),
       ('16551b42-292f-4316-800c-880c864fcbfd', 2, '3ce42a81-ae99-42b6-b4ed-d0dc1107e651');


drop schema if exists syncd2 cascade;
create schema syncd2;
create table syncd2.artists
(
    id      varchar not null primary key,
    name    varchar not null,
    version text    not null
);
create index artists__id_version_index on syncd2.artists (id, version);

create table syncd2.performers
(
    id      varchar not null primary key,
    name    varchar not null,
    label   varchar not null,
    version text    not null
);
create index performers__id_version on syncd2.performers (id, version);
create index performers__label on syncd2.performers (label);

create table syncd2.performer_artists
(
    performer varchar not null references syncd2.performers,
    artist    varchar not null
);
create unique index performer_artists__performer_artist on syncd2.performer_artists (performer, artist);
create index performer_artists__artist on syncd2.performer_artists (artist);

create table syncd2.songs
(
    id        varchar not null primary key,
    name      varchar not null,
    composer  varchar not null,
    publisher varchar not null,
    version   text    not null
);
create index songs__id_version on syncd2.songs (id, version);
create index songs__composer on syncd2.songs (composer);
create index songs__publisher on syncd2.songs (publisher);

create table syncd2.performances
(
    performer varchar not null references syncd2.performers (id),
    sequence  int     not null,
    song      varchar not null
);
create unique index performances__performer_sequence on syncd2.performances (performer, sequence);
create index performances_song on syncd2.performances (song);
