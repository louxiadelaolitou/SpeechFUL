part of 'post_screens.dart';

enum _ComposerInputTarget { title, body }

class _ComposerDock extends StatelessWidget {
  const _ComposerDock({
    required this.type,
    required this.emojiOpen,
    required this.enabled,
    required this.canAddImages,
    required this.saving,
    required this.saved,
    required this.status,
    required this.error,
    required this.onTypeChanged,
    required this.onToggleEmoji,
    required this.onEmojiSelected,
    required this.onAddImages,
  });

  final PostComposerType type;
  final bool emojiOpen, enabled, canAddImages;
  final bool saving, saved;
  final String status;
  final String? error;
  final ValueChanged<PostComposerType> onTypeChanged;
  final VoidCallback onToggleEmoji, onAddImages;
  final ValueChanged<String> onEmojiSelected;

  @override
  Widget build(BuildContext context) {
    final palette = HhTelegramPalette.of(context);
    return DecoratedBox(
      decoration: BoxDecoration(
        color: palette.surface,
        border: Border(top: BorderSide(color: palette.line)),
      ),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Padding(
            padding: const EdgeInsets.fromLTRB(16, 7, 16, 3),
            child: Semantics(
              liveRegion: error != null,
              child: Row(
                children: [
                  Icon(
                    error != null
                        ? Icons.error_outline
                        : saving
                        ? Icons.cloud_sync_outlined
                        : saved
                        ? Icons.cloud_done_outlined
                        : Icons.cloud_upload_outlined,
                    size: 16,
                    color: error == null ? palette.secondary : palette.error,
                  ),
                  const SizedBox(width: 6),
                  Expanded(
                    child: Text(
                      error ?? status,
                      maxLines: 2,
                      overflow: TextOverflow.ellipsis,
                      style: TextStyle(
                        fontSize: 12,
                        height: 1.35,
                        color: error == null
                            ? palette.secondary
                            : palette.error,
                      ),
                    ),
                  ),
                ],
              ),
            ),
          ),
          Padding(
            padding: const EdgeInsets.fromLTRB(12, 3, 8, 8),
            child: Row(
              children: [
                Expanded(
                  child: Semantics(
                    container: true,
                    label: hhText(context, '帖子类型'),
                    child: SegmentedButton<PostComposerType>(
                      key: const ValueKey('post-composer-type'),
                      segments: [
                        ButtonSegment(
                          value: PostComposerType.imageText,
                          label: Text(hhText(context, '图文')),
                          icon: const Icon(Icons.photo_library_outlined),
                        ),
                        ButtonSegment(
                          value: PostComposerType.text,
                          label: Text(hhText(context, '文字')),
                          icon: const Icon(Icons.notes_outlined),
                        ),
                      ],
                      selected: {type},
                      showSelectedIcon: false,
                      onSelectionChanged: enabled
                          ? (selection) => onTypeChanged(selection.single)
                          : null,
                      style: ButtonStyle(
                        minimumSize: const WidgetStatePropertyAll(Size(0, 44)),
                        tapTargetSize: MaterialTapTargetSize.padded,
                        visualDensity: VisualDensity.compact,
                        textStyle: WidgetStatePropertyAll(
                          Theme.of(context).textTheme.labelLarge
                              ?.copyWith(height: 1.2),
                        ),
                      ),
                    ),
                  ),
                ),
                if (canAddImages)
                  IconButton(
                    tooltip: hhText(context, '添加图片'),
                    onPressed: enabled ? onAddImages : null,
                    icon: const Icon(Icons.add_photo_alternate_outlined),
                  ),
                IconButton(
                  key: const ValueKey('post-composer-emoji-toggle'),
                  tooltip: hhText(context, emojiOpen ? '键盘' : '表情'),
                  isSelected: emojiOpen,
                  style: IconButton.styleFrom(
                    backgroundColor: emojiOpen
                        ? palette.selection
                        : Colors.transparent,
                    foregroundColor: emojiOpen
                        ? palette.accent
                        : palette.secondary,
                  ),
                  onPressed: enabled ? onToggleEmoji : null,
                  icon: Icon(
                    emojiOpen
                        ? Icons.keyboard_outlined
                        : Icons.sentiment_satisfied_alt_outlined,
                  ),
                ),
              ],
            ),
          ),
          AnimatedSize(
            duration: HhMotion.duration(context, HhMotion.standard),
            curve: HhMotion.curve,
            alignment: Alignment.topCenter,
            child: emojiOpen
                ? _EmojiPanel(
                    height:
                        (MediaQuery.sizeOf(context).height -
                                MediaQuery.viewInsetsOf(context).bottom -
                                kToolbarHeight -
                                160)
                            .clamp(0.0, 280.0),
                    onSelected: onEmojiSelected,
                  )
                : const SizedBox(width: double.infinity),
          ),
        ],
      ),
    );
  }
}

class _EmojiPanel extends StatefulWidget {
  const _EmojiPanel({required this.height, required this.onSelected});

  final double height;
  final ValueChanged<String> onSelected;

  @override
  State<_EmojiPanel> createState() => _EmojiPanelState();
}

class _EmojiPanelState extends State<_EmojiPanel> {
  int _category = 0;

  static const _categories = <({String label, List<String> emojis})>[
    (
      label: '常用',
      emojis: [
        '😀',
        '😂',
        '🥹',
        '😊',
        '😍',
        '😘',
        '😎',
        '🤔',
        '😭',
        '😤',
        '😴',
        '🙃',
        '😅',
        '🤗',
        '😢',
        '😡',
        '🫠',
        '🥳',
        '✨',
        '👀',
        '👍',
        '👏',
        '🙏',
        '💪',
        '🎉',
        '❤️',
        '🧡',
        '💛',
        '💚',
        '💙',
        '💜',
        '🖤',
        '🤍',
        '💯',
        '🔥',
      ],
    ),
    (
      label: '心情',
      emojis: [
        '😄',
        '😆',
        '😉',
        '😋',
        '🤩',
        '🥰',
        '😇',
        '😌',
        '😔',
        '😞',
        '😕',
        '😐',
        '😶',
        '😬',
        '😮‍💨',
        '😱',
        '😵',
        '💭',
        '💤',
        '🌙',
        '☀️',
        '🌧️',
        '🌈',
        '🍀',
        '🌸',
        '🌻',
        '🫶',
        '👌',
        '🤝',
        '🙌',
        '🤞',
        '✌️',
      ],
    ),
    (
      label: '校园',
      emojis: [
        '🎓',
        '📚',
        '✏️',
        '🖊️',
        '📝',
        '📖',
        '🏫',
        '🧑‍🎓',
        '👩‍🎓',
        '👨‍🎓',
        '🍜',
        '🍚',
        '☕',
        '🥤',
        '🚲',
        '🏃',
        '⚽',
        '🏀',
        '🎧',
        '🎵',
        '🎬',
        '📷',
        '🎮',
        '🕹️',
        '🌳',
        '📅',
        '📌',
        '💡',
        '🧩',
        '🛟',
      ],
    ),
  ];

  @override
  Widget build(BuildContext context) {
    final palette = HhTelegramPalette.of(context);
    final emojis = _categories[_category].emojis;
    return Container(
      key: const ValueKey('post-composer-emoji-panel'),
      height: widget.height,
      decoration: BoxDecoration(
        color: palette.background,
        border: Border(top: BorderSide(color: palette.line)),
      ),
      child: LayoutBuilder(
        builder: (context, constraints) {
          final showCategories = constraints.maxHeight >= 132;
          return Column(
            children: [
              Expanded(
                child: GridView.builder(
                  padding: EdgeInsets.fromLTRB(
                    12,
                    8,
                    12,
                    showCategories ? 12 : 4,
                  ),
                  gridDelegate: const SliverGridDelegateWithFixedCrossAxisCount(
                    crossAxisCount: 7,
                    mainAxisExtent: 48,
                  ),
                  itemCount: emojis.length,
                  itemBuilder: (context, index) {
                    final emoji = emojis[index];
                    final label = hhText(
                      context,
                      '插入表情 {v0}',
                      args: {'v0': emoji},
                    );
                    return Semantics(
                      button: true,
                      label: label,
                      child: Tooltip(
                        message: label,
                        child: InkWell(
                          key: ValueKey('post-composer-emoji-$emoji'),
                          borderRadius: BorderRadius.circular(8),
                          onTap: () => widget.onSelected(emoji),
                          child: Center(
                            child: Text(
                              emoji,
                              style: const TextStyle(
                                fontSize: 24,
                                height: 1,
                                fontFamilyFallback: [
                                  'Noto Color Emoji',
                                  'Apple Color Emoji',
                                  'Segoe UI Emoji',
                                ],
                              ),
                            ),
                          ),
                        ),
                      ),
                    );
                  },
                ),
              ),
              if (showCategories)
                SizedBox(
                  height: 52,
                  child: ListView.separated(
                    padding: const EdgeInsets.fromLTRB(12, 4, 12, 4),
                    scrollDirection: Axis.horizontal,
                    itemCount: _categories.length,
                    separatorBuilder: (_, _) => const SizedBox(width: 8),
                    itemBuilder: (context, index) => ChoiceChip(
                      key: ValueKey('post-composer-emoji-category-$index'),
                      label: Text(hhText(context, _categories[index].label)),
                      selected: _category == index,
                      onSelected: (_) => setState(() => _category = index),
                      visualDensity: VisualDensity.compact,
                    ),
                  ),
                ),
            ],
          );
        },
      ),
    );
  }
}
