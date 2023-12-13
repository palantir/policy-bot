const tailwind = require('tailwindcss');
const purgecss = require('@fullhuman/postcss-purgecss');
const cssnano = require('cssnano');

const isProduction = process.env.NODE_ENV === 'production';

module.exports = {
  plugins: [
    tailwind('./tailwind.config.js'),
    ...(isProduction ? [
      purgecss({
        content: [
          './server/templates/**/*.html.tmpl',
        ],
        defaultExtractor: content => {
          return content.match(/[\w-./]*\w/g) || [];
        },
        safelist: [
          // status classes are dynamically generated
          "approved", "disapproved", "pending", "skipped", "error",
          // keep all selectors that relate to hidden statuses
          "data-next-status", "data-hide-status",
        ],
      }),
      cssnano({
        preset: ['default', {
          // this breaks some tailwind utils, like directional borders
          mergeLonghand: false,
        }],
      }),
    ] : []),
  ],
};
