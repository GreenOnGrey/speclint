# Design specification: <feature title>

The gate is needed if the change adds or changes screens, their states or texts. Otherwise the writer proposes to skip it (`FTR.HMR.DEV-0001` R6).

## Visual language

## Layout

## Screens and states

For every screen: purpose, content, states (empty, loading, error, no access) and the requirements of the product specification it covers (`R<n>`).

## Mockups

Links to the mockup files in the folder of the area. The search of specifications does not read mockups: everything important about screens and states is in the text above.

An HTML mockup declares `color-scheme` in each theme (`light` in the light one, `dark` in the dark one): the navigator shows it in an isolated frame, and the browser draws the scroll bars and form fields of the mockup by this scheme, not Hammurapi.

The initial theme of a mockup comes from the fragment `#theme=light` or `#theme=dark` of its address, and without it from `prefers-color-scheme`: this is how Hammurapi opens the mockup in the right theme without entering it. The mockup keeps its own theme switch.

## Decisions

| # | Decision | Reason |
|---|---|---|
