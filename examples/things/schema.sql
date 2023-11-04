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

create schema if not exists things;
create table things.things
(
    id          varchar not null primary key,
    owner       varchar not null,
    name        varchar not null,
    actionCount integer default 0,
    version     text    not null
);
create index things__id_version_index on things.things (id, version);

create table things.actions
(
    thing    varchar not null,
    action   varchar not null,
    time     integer not null,
    sequence integer not null
);
create unique index things__things_sequence on things.actions (thing, sequence);
