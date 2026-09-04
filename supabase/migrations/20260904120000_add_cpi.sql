create table public.cpi (
  date date primary key,
  value double precision not null check (value > 0),
  source_url text not null
);

alter table public.cpi enable row level security;
