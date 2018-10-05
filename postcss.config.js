const tailwind = require('tailwindcss');
const purgecss = require('@fullhuman/postcss-purgecss');
const cssnano = require('cssnano');

const isProduction = process.env.NODE_ENV === 'production';

// from https://tailwindcss.com/docs/controlling-file-size#removing-unused-css-with-purgecss
// needed to handle special characters in class names
class TailwindExtractor {
  static extract(content) {
    return content.match(/[A-Za-z0-9-_:\/]+/g) || [];
  }
}

module.exports = {
  plugins: [
    tailwind('./tailwind.config.js'),
    ...(isProduction ? [
      purgecss({
        content: [
          './server/templates/**/*.html.tmpl',
        ],
        extractors: [{
          extractor: TailwindExtractor,
          extensions: ["html.tmpl"],
        }],
        // status classes are dynamically generated
        whitelist: [
          "approved", "disapproved", "pending", "skipped", "error",
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
