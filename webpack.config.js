const path = require('path');
const MiniCssExtractPlugin = require('mini-css-extract-plugin');

const isProduction = process.env.NODE_ENV === "production";
const hashStr = isProduction ? '-[contenthash:10]' : '';

// webpack-manifest-plugin v6 ships as ESM, so it can only be loaded with a
// dynamic import() from this CommonJS config. It is only needed for the
// content-hashed production build, so load it lazily there. Exporting an async
// function lets webpack await the import before building.
module.exports = async () => {
  const plugins = [
    new MiniCssExtractPlugin({
      filename: `css/[name]${hashStr}.css`,
    }),
  ];

  if (isProduction) {
    const { WebpackManifestPlugin } = await import('webpack-manifest-plugin');
    plugins.push(new WebpackManifestPlugin({
      publicPath: '',
      generate: (seed, files, entrypoints) => {
        return files.reduce(
          (manifest, file) => {
            const key = path.join(path.dirname(file.path), path.basename(file.name));
            return Object.assign(manifest, { [key]: file.path })
          },
          seed
        );
      },
    }));
  }

  return {
  mode: isProduction ? 'production' : 'development',
  entry: {
    main: './server/assets/index.js',
    htmx: 'htmx.org',
  },
  output: {
    filename: `js/[name]${hashStr}.js`,
    path: path.resolve(__dirname, 'build', 'static'),
    clean: true,
  },
  plugins,
  module: {
    rules: [
      {
        test: /\.css$/i,
        use: [MiniCssExtractPlugin.loader, 'css-loader', 'postcss-loader'],
      },
      {
        test: /\.(ico|svg)$/i,
        type: 'asset/resource',
        generator: {
          filename: `img/[name]${isProduction ? '-[hash:10]' : ''}[ext]`,
        },
      },
    ]
  },
  };
}
