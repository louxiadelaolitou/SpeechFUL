import 'package:flutter/material.dart';

import 'student_letter_dialog.dart';
import 'student_letter_store.dart';

/// Blocks the signed-out entry screen until this installation acknowledges the
/// letter. Authenticated routes do not mount this gate.
class StudentLetterGate extends StatefulWidget {
  const StudentLetterGate({
    super.key,
    required this.store,
    required this.child,
  });

  final StudentLetterStore store;
  final Widget child;

  @override
  State<StudentLetterGate> createState() => _StudentLetterGateState();
}

class _StudentLetterGateState extends State<StudentLetterGate> {
  bool _checked = false, _ready = false, _queued = false;
  DialogRoute<void>? _dialog;

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    var read = false;
    try {
      read = await widget.store.hasRead();
    } catch (_) {
      // A missing/unreadable receipt leaves this letter unread.
    }
    if (!mounted) return;
    setState(() {
      _checked = true;
      _ready = read;
    });
    if (!read) {
      _schedule();
    }
  }

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    _schedule();
  }

  void _schedule() {
    if (!_checked || _ready || _queued || _dialog != null) return;
    _queued = true;
    WidgetsBinding.instance.addPostFrameCallback((_) async {
      _queued = false;
      if (!mounted ||
          _ready ||
          _dialog != null ||
          ModalRoute.isCurrentOf(context) != true) {
        return;
      }
      var acknowledged = false;
      final route = DialogRoute<void>(
        context: context,
        barrierDismissible: false,
        builder: (_) => StudentLetterDialog(
          onRead: () async {
            await widget.store.markRead();
            acknowledged = true;
          },
        ),
      );
      _dialog = route;
      await Navigator.of(context).push(route);
      if (_dialog == route) _dialog = null;
      if (!mounted) return;
      if (!acknowledged) {
        _schedule();
        return;
      }
      setState(() => _ready = true);
    });
    WidgetsBinding.instance.ensureVisualUpdate();
  }

  @override
  void dispose() {
    final route = _dialog;
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (route?.isActive == true) route!.navigator?.removeRoute(route);
    });
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => ExcludeFocus(
    excluding: !_ready,
    child: AbsorbPointer(absorbing: !_ready, child: widget.child),
  );
}
